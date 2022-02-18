#!/usr/bin/env bash
# This is a wrapper around gobin.sh to run golangci-lint.
# Useful for using the correct version of golangci-lint
# with your editor.

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
GOBIN="$DIR/gobin.sh"

# shellcheck source=./lib/bootstrap.sh
source "$DIR/lib/bootstrap.sh"

if [[ -z $workspaceFolder ]]; then
  workspaceFolder="$(get_repo_directory)"
fi

# Enable only fast linters, and always use the correct config.
args=("--config=${workspaceFolder}/scripts/golangci.yml" "$@" "--fast" "--allow-parallel-runners")

# trade memory for CPU when running the linters
export GOGC=20

version="$(get_application_version "golangci-lint")"
if grep "^\d" <<<"$version"; then
  version="v$version"
fi

exec "$GOBIN" "github.com/jaredallard/golangci-lint/cmd/golangci-lint@$version" "${args[@]}"
