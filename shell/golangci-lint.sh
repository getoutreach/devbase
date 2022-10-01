#!/usr/bin/env bash
# This is a wrapper around gobin.sh to run golangci-lint.
# Useful for using the correct version of golangci-lint
# with your editor.

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"

# shellcheck source=./lib/bootstrap.sh
source "$DIR/lib/bootstrap.sh"
# shellcheck source=./lib/asdf.sh
source "$DIR/lib/asdf.sh"

if [[ -z $workspaceFolder ]]; then
  workspaceFolder="$(get_repo_directory)"
fi

# Enable only fast linters, and always use the correct config.
args=("--config=${workspaceFolder}/scripts/golangci.yml" "$@" "--fast" "--allow-parallel-runners")

# trade memory for CPU when running the linters
export GOGC=20

# Use individual directories for golangci-lint cache as opposed to a mono-directory.
# This helps with the "too many open files" error.
mkdir -p "$HOME/.outreach/.cache/.golangci-lint" >/dev/null 2>&1

# Why: We're OK with masking the return value
# shellcheck disable=SC2155
export GOLANGCI_LINT_CACHE="$HOME/.outreach/.cache/.golangci-lint/$(get_app_name)"

asdf_devbase_exec golangci-lint "${args[@]}"
