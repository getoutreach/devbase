#!/usr/bin/env bash
# Runs lintroller with the repository's golangci-lint config file.
set -euo pipefail

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"

# shellcheck source=./lib/mise/stub.sh
source "$DIR/lib/mise/stub.sh"

export GOFLAGS=-tags=or_e2e,or_test

if [[ -z ${workspaceFolder:-} ]]; then
  workspaceFolder="$(get_repo_directory)"
fi

args=(-config "$workspaceFolder/scripts/golangci.yml" "$@")

run_lintroller() {
  local customLintroller="$workspaceFolder/scripts/lintroller.sh"
  if [[ -x $customLintroller ]]; then
    "$customLintroller" "${args[@]}"
  else
    # The sed is used to strip the pwd from lintroller output,
    # which is currently prefixed with it.
    mise_exec_tool_with_bin github:getoutreach/lintroller \
      lintroller "${args[@]}"
  fi
}

run_lintroller 2>&1 | sed "s#^$(pwd)/##"
