#!/usr/bin/env bash
# Runs the formatter() function defined by a single linter script (see
# shell/linters/*.sh) against the files it declares via `extensions`.
#
# Used by shell/fmt.sh (in series) and by the per-formatter mise tasks
# under .mise/tasks/fmt/ (which mise may run in parallel).
set -e -o pipefail

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"

# shellcheck source=./lib/bootstrap.sh
source "$DIR/lib/bootstrap.sh"
# shellcheck source=./lib/github.sh
source "$DIR/lib/github.sh"
# shellcheck source=./lib/logging.sh
source "$DIR/lib/logging.sh"
# shellcheck source=./lib/mise.sh
source "$DIR/lib/mise.sh"
# shellcheck source=./lib/shell.sh
source "$DIR/lib/shell.sh"
# shellcheck source=./lib/version.sh
source "$DIR/lib/version.sh"

linterScript="${1:-}"
if [[ -z $linterScript ]]; then
  fatal "Usage: run-formatter.sh <path-to-linter-script>"
fi

# Note: extensions is set by the linter script sourced below.
extensions=()

# Why: Dynamic
# shellcheck disable=SC1090
source "$linterScript"

if [[ "$(find_files_with_extensions "${extensions[@]}" | wc -l | tr -d ' ')" -eq 0 ]]; then
  exit 0
fi

# Note: extensions is set by the linter.
extensionsPrefixed=("${extensions[@]/#/.}")
IFS=,
extensionsString="${extensionsPrefixed[*]}"
unset IFS

# show is used by run_command as metadata to be shown along with the command name
show=$extensionsString

# Set by the linter script.
if ! formatter; then
  error "Formatter failed to run"
  exit 1
fi
