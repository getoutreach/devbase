#!/usr/bin/env bash
#
# Helpers for finding the devbase version a repository requires, and for
# installing and running the devbase CLI. Assumes logging.sh and shell.sh
# are sourced.

_DEVBASE_LIB_DEVBASE_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"

# shellcheck source=./bootstrap.sh
source "${_DEVBASE_LIB_DEVBASE_DIR}/bootstrap.sh"
# shellcheck source=./mise.sh
source "${_DEVBASE_LIB_DEVBASE_DIR}/mise.sh"
# shellcheck source=./version.sh
source "${_DEVBASE_LIB_DEVBASE_DIR}/version.sh"

# devbase_tree_root echoes the root of the devbase source tree that
# bootstrap.sh belongs to: devbase's own repository, or a consumer
# repository's .bootstrap directory. It does not use
# get_devbase_directory, because that reads service.yaml through $YQ,
# and shell/yq.sh calls devbase_cli, which calls this function.
devbase_tree_root() {
  echo "$DEVBASE_DIR"
}

# devbase_cli_version echoes the devbase version the repository
# requires. It echoes "local" when this repository is devbase itself or
# when .version says "local". Otherwise it echoes the tag in .version,
# which scripts/devbase.sh wrote when it cloned .bootstrap.
devbase_cli_version() {
  local tree_root repo_root
  tree_root="$(devbase_tree_root)"
  repo_root="$(get_repo_directory)"

  if [[ $tree_root == "$repo_root" ]]; then
    echo "local"
    return
  fi
  cat "$tree_root/.version" 2>/dev/null || echo "local"
}

# devbase_cli_is_tagged_version succeeds if the version from
# devbase_cli_version is a release tag rather than "local".
devbase_cli_is_tagged_version() {
  [[ $1 =~ ^v[0-9] ]]
}

# devbase_mise_tool echoes the mise tool for installing the devbase CLI,
# without a version. Append "@<version>" without the "v" prefix. It uses
# the http backend, which downloads the release archive directly
# instead of making GitHub API requests.
devbase_mise_tool() {
  echo 'http:devbase[url=https://github.com/getoutreach/devbase/releases/download/v{{ version }}/devbase_{{ version }}_{{ os(macos="darwin") }}_{{ arch(x64="amd64") }}.tar.gz]'
}

# devbase_tree_is_dirty succeeds if the git tree at the given directory has
# uncommitted changes. It ignores the deleted service.yaml that
# scripts/devbase.sh leaves in non-local .bootstrap clones.
devbase_tree_is_dirty() {
  local treeRoot="$1"
  [[ -n "$(git -C "$treeRoot" status --porcelain 2>/dev/null | grep -vx ' D service.yaml' || true)" ]]
}

# devbase_version_from_git echoes the Go module version of the git
# checkout at the given directory: the tag if HEAD is on one, otherwise a
# pseudo-version built as the go command builds it. It appends "+dirty"
# for uncommitted changes, and fails if the directory is not a checkout.
devbase_version_from_git() {
  local dir="$1"
  local hash ts tag version

  hash="$(git -C "$dir" rev-parse --verify --short=12 HEAD 2>/dev/null)" || return 1
  ts="$(TZ=UTC git -C "$dir" show -s --format=%cd --date=format-local:%Y%m%d%H%M%S HEAD)"

  # Like the go command, use the highest reachable tag. versionsort.suffix
  # sorts v1.0.0-rc.1 before v1.0.0.
  tag="$(git -C "$dir" -c versionsort.suffix=- tag --merged HEAD --list 'v[0-9]*' --sort=-v:refname | head -n1)"

  if [[ -z $tag ]]; then
    version="v0.0.0-${ts}-${hash}"
  elif [[ $(git -C "$dir" rev-parse "$tag^{commit}") == "$(git -C "$dir" rev-parse HEAD)" ]]; then
    version="$tag"
  elif [[ $tag == *-* ]]; then
    version="${tag%%+*}.0.${ts}-${hash}"
  else
    local major minor patch
    IFS=. read -r major minor patch <<<"${tag#v}"
    version="v${major}.${minor}.$((${patch%%+*} + 1))-0.${ts}-${hash}"
  fi

  if devbase_tree_is_dirty "$dir"; then
    version="${version}+dirty"
  fi
  echo "$version"
}

# devbase_build_from_source builds devbase from this source tree to the
# given path. It sets gobox's app.Version from git when it can. It
# returns non-zero after printing an error if the build fails.
devbase_build_from_source() {
  local output="$1"
  local treeRoot goPath version
  treeRoot="$(devbase_tree_root)"
  # find_tool also finds a go that mise manages but that is not on PATH.
  goPath="$(find_tool go 2>/dev/null || true)"

  local ldflags=()
  if version="$(devbase_version_from_git "$treeRoot")"; then
    ldflags=(-ldflags "-X github.com/getoutreach/gobox/pkg/app.Version=${version}")
  fi

  PATH="${goPath:+$(dirname "$goPath"):}$PATH" go -C "$treeRoot" build -o "$output" ${ldflags[@]+"${ldflags[@]}"} ./cmd/devbase || {
    error "failed to build devbase from $treeRoot"
    return 1
  }
}

# devbase_dirty_tree_hash hashes the uncommitted content of the git tree
# at the given directory: changes against HEAD (staged or not, including
# binary diffs) and untracked, non-ignored files. It runs one `cat` per
# untracked file because `xargs -0 cat` with empty input reads stdin, and
# GNU (Linux) and BSD (macOS) xargs differ in that case.
devbase_dirty_tree_hash() {
  local treeRoot="$1"
  {
    git -C "$treeRoot" diff HEAD --binary
    git -C "$treeRoot" ls-files --others --exclude-standard -z |
      while IFS= read -r -d '' f; do cat "$treeRoot/$f"; done
  } | sha256sum | awk '{ print $1 }'
}

# devbase_tree_has_external_build_inputs succeeds if the go module at the
# given directory can build from files outside it, through a go.work file
# or a go.mod replace with a local path.
devbase_tree_has_external_build_inputs() {
  local dir="$1"

  if [[ ${GOWORK:-} != off ]]; then
    if [[ -n ${GOWORK:-} ]]; then
      return 0
    fi
    local parent="$dir"
    while [[ $parent != / && $parent != . ]]; do
      [[ -e $parent/go.work ]] && return 0
      parent="$(dirname "$parent")"
    done
  fi

  grep -Eq '=>[[:space:]]*(\.|/)' "$dir/go.mod" 2>/dev/null
}

# devbase_prune_source_binaries removes cached source binaries that were
# built more than DEVBASE_SOURCE_CACHE_MAX_AGE_DAYS (default 14) days ago.
# It prunes by age because the cache is shared by every source tree on the
# machine, so keeping only the newest entry would evict other trees.
devbase_prune_source_binaries() {
  local dir
  dir="$(dirname "$(dirname "$(cached_binary_path devbase-src x)")")"
  [[ -d $dir ]] || return 0
  find "$dir" -mindepth 1 -maxdepth 1 -type d \
    -mtime "+${DEVBASE_SOURCE_CACHE_MAX_AGE_DAYS:-14}" -exec rm -rf {} + 2>/dev/null || true
}

# devbase_cached_source_binary echoes the path to a devbase binary built
# from this source tree. It builds only if the cache has no binary for the
# tree's state. The cache key is the current commit, plus a hash of the
# uncommitted changes if the tree is dirty. The key cannot cover files
# outside the tree, so a tree with such inputs rebuilds on every call, into a
# binary path that is specific to the tree's path. The build is moved into
# place so that parallel callers never run a partial binary. Callers that run
# devbase many times (yq_wrapper, devbase_cli) use it instead of
# devbase_build_from_source.
devbase_cached_source_binary() {
  local treeRoot rev cacheKey cached tmp rebuild=false
  treeRoot="$(devbase_tree_root)"
  rev="$(git -C "$treeRoot" rev-parse HEAD)" || fatal "failed to resolve $treeRoot's current commit"
  if devbase_tree_has_external_build_inputs "$treeRoot"; then
    cacheKey="external-inputs-$(printf '%s' "$treeRoot" | sha256sum | cut -c1-16)"
    rebuild=true
  elif devbase_tree_is_dirty "$treeRoot"; then
    # A clean tree skips devbase_dirty_tree_hash, which costs more.
    cacheKey="dirty-$(devbase_dirty_tree_hash "$treeRoot")-${rev}"
  else
    cacheKey="$rev"
  fi

  cached="$(cached_binary_path devbase-src "$cacheKey")"
  if [[ $rebuild == true || ! -e $cached ]]; then
    mkdir -p "$(dirname "$cached")"
    tmp="$(mktemp "$cached.XXXXXX")"
    if ! devbase_build_from_source "$tmp"; then
      rm -f "$tmp"
      exit 1
    fi
    chmod +x "$tmp"
    mv -f "$tmp" "$cached"
    devbase_prune_source_binaries
  fi
  echo "$cached"
}

# devbase_install_from_source builds devbase from this source tree into
# the given directory, along with the release's yq and tomlq shims. It needs go,
# since a dev version has no release archive to install.
devbase_install_from_source() {
  local binDir="$1"

  find_tool go >/dev/null 2>&1 || fatal "using dev version of devbase, needs go installed"

  mkdir -p "$binDir"
  devbase_build_from_source "$binDir/devbase" || exit 1
  local shim
  for shim in yq tomlq; do
    install -m 0755 "$(devbase_tree_root)/release/$shim" "$binDir/$shim"
  done
}

# devbase_cli_download_release installs the devbase release for the
# given tag (for example, "v2.40.0") with mise and echoes the binary's
# path. devbase_cli_binary calls it only when no devbase of that
# version is installed.
devbase_cli_download_release() {
  local devbaseTool devbaseVersion
  devbaseTool="$(devbase_mise_tool)"
  devbaseVersion="${1#v}"

  # stdout is the binary's path, so send the install's output to stderr.
  install_tool_with_mise "$devbaseTool" "$devbaseVersion" >&2

  local binary
  if ! binary="$(run_mise which --tool "$devbaseTool@$devbaseVersion" devbase)" || [[ -z $binary ]]; then
    fatal "devbase $devbaseVersion was installed, but its binary was not found"
  fi
  echo "$binary"
}

# devbase_run runs the devbase binary at the given path with --skip-update
# and the rest of the arguments. In CI it also sets
# GOBOX_SKIP_VALIDATE_AUTH, as the binary's GitHub auth check can fall
# back to an interactive `gh auth login` that CI can't answer.
devbase_run() {
  local bin="$1"
  shift

  if in_ci_environment; then
    GOBOX_SKIP_VALIDATE_AUTH=true "$bin" --skip-update "$@"
  else
    "$bin" --skip-update "$@"
  fi
}

# devbase_cli_binary echoes the path to a devbase binary for the given
# tag. It uses the devbase that find_tool resolves if its version
# matches exactly, and installs the release otherwise.
devbase_cli_binary() {
  local version="$1"

  local existing
  existing="$(find_tool devbase 2>/dev/null || true)"
  if [[ -n $existing ]]; then
    # Compare exactly: a substring match would accept v2.41.0-rc.1 for
    # v2.41.0.
    local existing_version
    existing_version="$(devbase_run "$existing" --version 2>/dev/null | awk '{print $NF}')"
    if [[ $existing_version == "$version" ]]; then
      echo "$existing"
      return
    fi
  fi

  devbase_cli_download_release "$version"
}

# devbase_cli runs any devbase subcommand. It uses the release binary
# for a tagged version, and a build from source otherwise.
devbase_cli() {
  local version
  version="$(devbase_cli_version)"

  if ! devbase_cli_is_tagged_version "$version"; then
    local bin
    # Do not use `go run -C`: it also changes the binary's working
    # directory, which breaks relative path arguments.
    bin="$(devbase_cached_source_binary)" || exit 1
    devbase_run "$bin" "$@"
    exit $?
  fi

  # Replace this shell with devbase, so signals reach it directly. That
  # rules out devbase_run, so this repeats its CI check.
  if in_ci_environment; then
    export GOBOX_SKIP_VALIDATE_AUTH=true
  fi
  exec "$(devbase_cli_binary "$version")" --skip-update "$@"
}
