#!/usr/bin/env bash
# This is a wrapper around `mise` to run `golangci-lint`.
# Useful for using the correct version of golangci-lint
# with your editor.

set -eo pipefail

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

if [[ -z $workspaceFolder ]]; then
  workspaceFolder="$(get_repo_directory)"
fi

# Ensure that the configuration comes from the repo and not devbase.
args=("--config=${workspaceFolder}/scripts/golangci.yml" "$@")
args+=("--allow-parallel-runners" "--color=always" "--show-stats")

if in_ci_environment; then
  TEST_DIR="${workspaceFolder}/bin"
  TEST_FILENAME="${TEST_DIR}/golangci-lint-tests.xml"
  mkdir -p "$TEST_DIR"
  # Support multiple output formats (stdout, JUnit)
  args+=("--output.junit-xml.path=${TEST_FILENAME}" "--output.junit-xml.extended")
fi

# Determine the version of Go and golangci-lint to calculate compatibility.
GO_VERSION=$(go version | awk '{print $3}' | sed 's/go//')
GOLANGCI_LINT_VERSION=$(mise_exec_tool golangci-lint --version | awk '{print $4}')
# Only update this if something in devbase requires a specific version.
# For example, if the config schema has a breaking change, or the templated
# config file references a linter that was recently introduced.
MIN_GOLANGCI_LINT_VERSION="2.7.2"

if ! has_minimum_version "$MIN_GOLANGCI_LINT_VERSION" "$GOLANGCI_LINT_VERSION"; then
  fatal "golangci-lint version ${GOLANGCI_LINT_VERSION} is not supported. Please upgrade to >= ${MIN_GOLANGCI_LINT_VERSION}"
fi

if has_minimum_version "1.26.0" "$GO_VERSION" && ! has_minimum_version "2.9.0" "$GOLANGCI_LINT_VERSION"; then
  fatal "Go 1.26 and later requires golangci-lint 2.9.0 or later"
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
      warn "Failed to determine system memory or under threshold. " \
        "Falling back to GOGC" >&2
      export GOGC=20
    else
      # Use mem as the memory target and ensure that we have 1GB of room.
      export GOMEMLIMIT="$((mem - RESERVED_MEMORY_IN_MIB))MiB"
    fi
  fi
fi

if [[ -z ${GOLANGCI_LINT_CACHE:-} ]]; then
  # Use individual directories for golangci-lint cache as opposed to a mono-directory.
  # This helps with the "too many open files" error.
  GOLANGCI_LINT_CACHE="$HOME/.outreach/.cache/.golangci-lint/$(get_app_name)"
  mkdir -p "$GOLANGCI_LINT_CACHE" >/dev/null 2>&1
  export GOLANGCI_LINT_CACHE
fi

mise_exec_tool golangci-lint "${args[@]}"

if in_ci_environment; then
  mv "$TEST_FILENAME" /tmp/test-results/
fi
