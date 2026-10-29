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
# devbase_cli_version (shell/lib/devbase.sh) reads.
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

# shellcheck source=./lib/devbase.sh
source "${_DEVBASE_CLI_LIB_DIR}/devbase.sh"

# Allow running this script directly (e.g. `shell/cli.sh yq .`), not
# just sourcing it. Strict mode is scoped to this branch, not the top
# of the file -- see the comment above _DEVBASE_CLI_DIR for why.
if [[ ${BASH_SOURCE[0]} == "${0}" ]]; then
  set -euo pipefail
  devbase_cli "$@"
fi
