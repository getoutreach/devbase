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

# Determine the version of go and golangci-lint to calculate compatibility.
GO_MINOR_VERSION=$(go version | awk '{print $3}' | sed 's/go//' | cut -d'.' -f1,2)
GOLANGCILINT_VERSION=$(asdf_devbase_run golangci-lint --version | awk '{print $4}')
GO_MINOR_VERSION_INT=${GO_MINOR_VERSION//./}
GOLANGCI_LINT_VERSION_INT=${GOLANGCILINT_VERSION//./}
GOLANGCI_LINT_VERSION_INT=${GOLANGCI_LINT_VERSION_INT//v/}

# Check version compatibility for golangci-lint/go 1.X.
if [[ ${GO_MINOR_VERSION_INT:0:1} -lt 2 ]] && [[ ${GOLANGCI_LINT_VERSION_INT:0:1} -lt 2 ]]; then
  # Go 1.20 requires >= golangci-lint 1.52.0
  if [[ $GO_MINOR_VERSION_INT == 120 ]] && [[ $GOLANGCI_LINT_VERSION_INT -lt 1520 ]]; then
    echo "Error: Go 1.20 requires golangci-lint 1.52.0 or newer (detected $GOLANGCILINT_VERSION)" >&2
    exit 1
  fi

  # Go 1.21 requires >= golangci-lint 1.54.1
  if [[ $GO_MINOR_VERSION_INT == 121 ]] && [[ $GOLANGCI_LINT_VERSION_INT -lt 1541 ]]; then
    echo "Error: Go 1.21 requires golangci-lint 1.54.1 or newer (detected $GOLANGCILINT_VERSION)" >&2
    exit 1
  fi
fi

# If GOGC or GOMEMLIMIT aren't set, we attempt to set them to better
# manage memory usage by the golangci-linter in CI.
if [[ -z $GOGC ]] && [[ -z $GOMEMLIMIT ]]; then

  # RESERVED_MEMORY_IN_MIB is the amount of memory we want to reserve for
  # other processes or overheads. This will not be used by the linter.
  RESERVED_MEMORY_IN_MIB=2048

  # If we're on a system with free or sysctl, set GOMEMLIMIT to a value
  # that's less than the max amount of RAM on the system. This helps
  # ensure that we don't go over the memory limit and get OOM killed.
  # This is mostly important for CI systems or other container
  # environments.
  if (command -v free || command -v sysctl) &>/dev/null; then
    if command -v free &>/dev/null; then
      mem="$(free -m | awk '/^Mem:/{print $2}')"
    elif command -v sysctl &>/dev/null; then
      mem=$((($(sysctl -n hw.memsize) / 1024) / 1024))
    fi

    # If we don't have enough memory to hit the reserve or we failed to
    # determine how much memory we have, fall back to setting GOGC --
    # which is relative to the amount of memory we have.
    if [[ $mem -lt $RESERVED_MEMORY_IN_MIB ]] || [[ -z $mem ]]; then
      # Failed to determine GOMEMLIMIT somehow. Fallback to GOGC.
      echo "Warning: Failed to determine system memory or under threshold. " \
        "Falling back to GOGC" >&2
      export GOGC=20
    else
      # Use mem as the memory target and ensure that we have 1GB of room.
      export GOMEMLIMIT="$((mem - RESERVED_MEMORY_IN_MIB))MiB"
    fi
  fi
fi

# Use individual directories for golangci-lint cache as opposed to a mono-directory.
# This helps with the "too many open files" error.
mkdir -p "$HOME/.outreach/.cache/.golangci-lint" >/dev/null 2>&1

asdf_devbase_exec golangci-lint "${args[@]}"
