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
args=("--config=${workspaceFolder}/scripts/golangci.yml" "$@"  "-v" "--fast" "--allow-parallel-runners")


asdf_devbase_exec golangci-lint --version

# If we're on a system with free, set GOMEMLIMIT to a value that's less
# than the max amount of RAM on the system. This helps ensure that we
# don't go over the memory limit and get OOM killed. This is mostly
# important for CI systems.
if command -v free &>/dev/null; then
  mem="$(free -m | awk '/^Mem:/{print $2}')"

  # Use mem as the memory target and ensure that we have 2GB of room.
  export GOMEMLIMIT="$((mem - 2048))MiB"

  echo "Note: Using $GOMEMLIMIT of memory for golangci-lint"
else
  # If we're on a system that doesn't include free, fallback to setting
  # GOGC to a decently safe value. This isn't perfect, but should
  # prevent us from getting OOMs in most cases.
  export GOGC=20
fi

# Use individual directories for golangci-lint cache as opposed to a mono-directory.
# This helps with the "too many open files" error.
mkdir -p "$HOME/.outreach/.cache/.golangci-lint" >/dev/null 2>&1

asdf_devbase_exec golangci-lint "${args[@]}"
