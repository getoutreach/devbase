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
# devbase_cli_version (shell/lib/bootstrap.sh) reads.
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
# shellcheck source=./lib/devbase.sh
source "${_DEVBASE_CLI_LIB_DIR}/devbase.sh"
# shellcheck source=./lib/mise.sh
source "${_DEVBASE_CLI_LIB_DIR}/mise.sh"

# devbase_cli_download_release installs the devbase release binary for
# version (a real git tag, e.g. "v2.40.0" or "v2.41.0-rc.1") globally
# with mise (see devbase_mise_tool), and echoes the binary's path. It
# does not check for an already-installed devbase: that is
# devbase_cli_binary's job, and it only calls this when the installed
# one is missing or is a different version.
devbase_cli_download_release() {
  local version="$1"

  # stdout is the binary's path, so send the install's output to stderr.
  install_tool_with_mise "$(devbase_mise_tool)" "${version#v}" >&2

  # Not find_tool: it prefers a devbase on PATH, which may be the
  # wrong version.
  run_mise which devbase
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

# Allow running this script directly (e.g. `shell/cli.sh yq .`), not
# just sourcing it. Strict mode is scoped to this branch, not the top
# of the file -- see the comment above _DEVBASE_CLI_DIR for why.
if [[ ${BASH_SOURCE[0]} == "${0}" ]]; then
  set -euo pipefail
  devbase_cli "$@"
fi
