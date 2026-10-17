#!/usr/bin/env bash
#
# Resolves and runs the `devbase` CLI for the current repository,
# choosing between a tagged release binary (stable or RC) and running
# from source (dev/local), depending on what version of devbase the
# current repository requires. This is generic to any devbase
# subcommand -- not yq-specific -- so it is a shared, sourced library
# (see devbase_cli below) rather than something duplicated per
# subcommand shim.
#
# A consumer repository's .bootstrap directory is never a compiled
# binary: scripts/devbase.sh only ever produces a source tree, either a
# `git clone --single-branch --branch "$version"` of a real tag (stable
# or RC -- both ordinary git refs) or, when stencil.lock's devbase
# version is "local", a symlink to the on-disk checkout named in
# service.yaml's `.replacements["github.com/getoutreach/devbase"]`. It
# records the resolved value in .bootstrap/.version, which is what
# devbase_cli_version reads below.
#
# This file does not set `set -euo pipefail` at the top level: `set`
# inside a sourced file changes the *sourcing* shell's own options for
# the rest of its life, not just this file's. shell/lib/yq.sh sources
# this file into long-lived library shells (bootstrap.sh, yaml.sh,
# etc.) that do not expect strict mode imposed on them from a
# dependency they didn't ask for. It is only set below, scoped to
# running this file directly as a script.

_DEVBASE_CLI_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
_DEVBASE_CLI_LIB_DIR="${_DEVBASE_CLI_DIR}/lib"

# shellcheck source=./lib/logging.sh
source "${_DEVBASE_CLI_LIB_DIR}/logging.sh"
# shellcheck source=./lib/shell.sh
source "${_DEVBASE_CLI_LIB_DIR}/shell.sh"

ensure_bash_5_or_greater

# shellcheck source=./lib/bootstrap.sh
source "${_DEVBASE_CLI_LIB_DIR}/bootstrap.sh"
# shellcheck source=./lib/mise.sh
source "${_DEVBASE_CLI_LIB_DIR}/mise.sh"

# devbase_tree_root echoes the root of the devbase source tree this
# script itself lives in: the real devbase repo root when this
# repository is devbase itself, or a consumer repository's .bootstrap
# clone/symlink of it otherwise. It is computed directly from this
# script's own location, deliberately not via
# shell/lib/bootstrap.sh's get_devbase_directory (or get_app_name,
# which it calls): both read service.yaml's name field through $YQ,
# which is shell/yq.sh -- and shell/yq.sh's own job, once it delegates
# to devbase_cli below, is to call this file. Going through $YQ here
# would recurse into the very yq invocation devbase_cli exists to
# resolve.
devbase_tree_root() {
  cd "$_DEVBASE_CLI_DIR/.." >/dev/null 2>&1 && pwd
}

# devbase_cli_version echoes the devbase version the current
# repository requires: "local" when devbase_tree_root is the current
# repository's own root (working on devbase itself -- never run
# against a tagged copy of itself while developing it) or its
# .version file says "local" (a consumer repo whose stencil.lock
# points at a local checkout); otherwise the real git tag
# scripts/devbase.sh cloned .bootstrap at.
devbase_cli_version() {
  local tree_root repo_root
  tree_root="$(devbase_tree_root)"
  repo_root="$(get_repo_directory)" # pure directory walk, no $YQ.

  if [[ $tree_root == "$repo_root" ]]; then
    echo "local"
    return
  fi
  cat "$tree_root/.version" 2>/dev/null || echo "local"
}

# devbase_cli_is_tagged_version returns success if version (as returned
# by devbase_cli_version) is a real git tag rather than "local"/dev.
# devbase_cli_version never returns a bare "local" that also matches
# ^v[0-9], so testing the regex alone already covers both cases.
devbase_cli_is_tagged_version() {
  [[ $1 =~ ^v[0-9] ]]
}

# devbase_cli_platform echoes "<os>_<arch>", matching goreleaser's own
# default archive naming (confirmed against real devbase release
# tarballs, e.g. devbase_2.40.0-rc.2_darwin_amd64.tar.gz).
#
# TODO(malept): shell/lib/buildx.sh has its own copy of this same
# x86_64/aarch64 -> amd64/arm64 mapping. There is no shared helper to
# call instead today (buildx.sh's own ARCH logic is private to that
# file, not exported), so fixing this means extracting a real helper
# (e.g. into shell/lib/shell.sh) and updating both call sites, not just
# this one -- deferred rather than done here since it touches a file
# with no other reason to change in this PR.
devbase_cli_platform() {
  local os arch
  os="$(uname -s | tr '[:upper:]' '[:lower:]')"
  arch="$(uname -m)"
  case "$arch" in
  x86_64) arch=amd64 ;;
  aarch64 | arm64) arch=arm64 ;;
  esac
  echo "${os}_${arch}"
}

# devbase_cli_download_release downloads and caches the devbase release
# binary for version (a real git tag, e.g. "v2.40.0" or
# "v2.41.0-rc.1"), mirroring shell/gobin.sh's own download-and-cache
# pattern (retry, a temp dir, shell/lib/shell.sh's cached_binary_path/
# get_cached_binary), and echoes the cached binary's path.
#
# Every step below checks its own exit status explicitly rather than
# relying on `set -e`: this file does not set it globally (see the
# comment above _DEVBASE_CLI_DIR), since sourcing it must not impose strict mode on
# a caller's own shell.
devbase_cli_download_release() {
  local version="$1"

  local cached
  cached="$(get_cached_binary devbase "$version")"
  if [[ -n $cached ]]; then
    echo "$cached"
    return
  fi
  cached="$(cached_binary_path devbase "$version")"

  local platform archive tmp_dir
  platform="$(devbase_cli_platform)"
  archive="devbase_${version#v}_${platform}.tar.gz"
  tmp_dir="$(mktemp -d)"

  if ! retry 5 5 curl --fail --location --silent --output "$tmp_dir/$archive" \
    "https://github.com/getoutreach/devbase/releases/download/$version/$archive"; then
    rm -rf "$tmp_dir"
    fatal "failed to download devbase $version release archive"
  fi

  if ! tar --directory="$tmp_dir" --extract --file="$tmp_dir/$archive" devbase; then
    rm -rf "$tmp_dir"
    fatal "failed to extract devbase $version release archive"
  fi

  if ! cp "$tmp_dir/devbase" "$cached" || ! chmod +x "$cached"; then
    rm -rf "$tmp_dir"
    fatal "failed to install devbase $version release binary to $cached"
  fi

  rm -rf "$tmp_dir"
  echo "$cached"
}

# devbase_cli_binary echoes the path to a devbase binary for version (a
# real git tag): an already-resolvable devbase (via find_tool, which
# checks PATH then a mise-managed install) if its own --version matches
# version exactly, otherwise a freshly downloaded-and-cached release
# binary.
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
# between a tagged release binary and `go run` from source depending on
# what version of devbase the current repository requires (see
# devbase_cli_version). `go run`'s own build cache makes the source
# path fast on repeat calls, so no separate binary cache is needed for
# it, unlike the tagged-release path.
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
    exec go -C "$(devbase_tree_root)" run ./cmd/devbase "$@"
  fi

  exec "$(devbase_cli_binary "$version")" "$@"
}

# Allow running this script directly (e.g. `shell/cli.sh yq .`), not
# just sourcing it. Strict mode is scoped to this branch, not the top
# of the file -- see the comment above _DEVBASE_CLI_DIR for why.
if [[ ${BASH_SOURCE[0]} == "${0}" ]]; then
  set -euo pipefail
  devbase_cli "$@"
fi
