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
# instead of making GitHub API requests, and checks the archive against
# the release's checksums.txt.
devbase_mise_tool() {
  local releaseURL="https://github.com/getoutreach/devbase/releases/download/v{{ version }}"
  echo "http:devbase[url=$releaseURL/devbase_{{ version }}_{{ os(macos=\"darwin\") }}_{{ arch(x64=\"amd64\") }}.tar.gz,checksum_url=$releaseURL/checksums.txt]"
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

  run_mise which --tool "$devbaseTool@$devbaseVersion" devbase
}

# devbase_cli_binary echoes the path to a devbase binary for version (a
# real git tag): an already-resolvable devbase (via find_tool, which
# checks PATH then a mise-managed install) if its own --version matches
# version exactly, otherwise a freshly installed release binary.
devbase_cli_binary() {
  local version="$1"

  local existing
  existing="$(find_tool devbase 2>/dev/null || true)"
  if [[ -n $existing ]]; then
    # cli/v3's own version output is "<name> version <version>"; take
    # the last field rather than substring-matching the whole line, so
    # a requested "v2.41.0" doesn't wrongly match an installed
    # "v2.41.0-rc.1" (a real devbase tag pattern) just because it
    # contains that substring.
    local existing_version
    existing_version="$("$existing" --version 2>/dev/null | awk '{print $NF}')"
    if [[ $existing_version == "$version" ]]; then
      echo "$existing"
      return
    fi
  fi

  devbase_cli_download_release "$version"
}

# devbase_cli runs the devbase CLI with the given arguments, resolving
# between a tagged release binary and a from-source build depending on
# what version of devbase the current repository requires (see
# devbase_cli_version).
#
# The from-source path builds to a temp binary rather than `exec go -C
# "$(devbase_tree_root)" run ./cmd/devbase "$@"`: `go run`'s `-C`
# changes the directory of the whole invocation, including the
# resulting binary's own working directory, not just where the module
# is resolved from. A caller like shell/lib/bootstrap.sh's
# stencil_version passes stencil.lock as a path relative to ITS OWN
# cwd, not devbase_tree_root's, so a devbase process that silently
# started in devbase_tree_root instead would look for that relative
# path in the wrong place. Building separately (cd'd into
# devbase_tree_root only for the build step, in a subshell) and then
# running the result from the caller's own, unchanged cwd keeps
# relative-path arguments correct. Go's package build cache still
# makes repeat calls fast; only the final link is repeated each time.
#
# This function is generic to any devbase subcommand: any script
# needing to run one (e.g. a future shell entry point for `lint
# graphql`, which has no shell wrapper today) should use this instead
# of duplicating the resolution logic -- see shell/yq.sh for the
# pattern.
devbase_cli() {
  local version
  version="$(devbase_cli_version)"

  if ! devbase_cli_is_tagged_version "$version"; then
    local bin status
    bin="$(mktemp)"
    if ! (cd "$(devbase_tree_root)" && go build -o "$bin" ./cmd/devbase); then
      rm -f "$bin"
      fatal "failed to build devbase from source"
    fi
    chmod +x "$bin"
    "$bin" "$@"
    status=$?
    rm -f "$bin"
    exit "$status"
  fi

  exec "$(devbase_cli_binary "$version")" "$@"
}
