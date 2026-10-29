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

# devbase_build_from_source builds devbase from this source tree to the
# given path. It returns non-zero after printing an error if the build
# fails.
devbase_build_from_source() {
  local output="$1"
  local treeRoot goPath
  treeRoot="$(devbase_tree_root)"
  # find_tool also finds a go that mise manages but that is not on PATH.
  goPath="$(find_tool go 2>/dev/null || true)"

  PATH="${goPath:+$(dirname "$goPath"):}$PATH" go -C "$treeRoot" build -o "$output" ./cmd/devbase || {
    error "failed to build devbase from $treeRoot"
    return 1
  }
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
    existing_version="$("$existing" --version 2>/dev/null | awk '{print $NF}')"
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
    local bin status
    bin="$(mktemp)"
    # Not `go run -C`: it would also change the binary's working
    # directory, which breaks relative path arguments.
    if ! devbase_build_from_source "$bin"; then
      rm -f "$bin"
      exit 1
    fi
    chmod +x "$bin"
    "$bin" "$@"
    status=$?
    rm -f "$bin"
    exit "$status"
  fi

  exec "$(devbase_cli_binary "$version")" "$@"
}
