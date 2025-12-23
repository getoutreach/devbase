#!/usr/bin/env bash
# Runs all files with .bats at the end of the filename
set -euo pipefail

# DIR is the directory of this script.
DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
DEVBASE_LIB_DIR="$DIR/../shell/lib"

# shellcheck source=../shell/lib/bootstrap.sh
source "$DEVBASE_LIB_DIR/bootstrap.sh"
# shellcheck source=../shell/lib/logging.sh
source "$DEVBASE_LIB_DIR/logging.sh"
# shellcheck source=../shell/lib/shell.sh
source "$DEVBASE_LIB_DIR/shell.sh"

# Check if the bats test helpers are installed and usable.
if [[ ! -f "$DIR/bats/test_helper/bats-assert/load.bash" ]]; then
  info "Initializing bats submodule(s) ..."
  git submodule update --init --recursive
fi

# Find all files with _test.sh at the end of the filename
# and run them
mapfile -t test_files < <(find_files_with_extensions "bats")

extraArgs=()
if in_ci_environment; then
  # If we're running in CI, we want to output junit test results.
  junitOutputPath="$(get_repo_directory)/bin/junit-test-results"
  mkdir -p "$junitOutputPath"

  extraArgs+=("--report-formatter" "junit" "--output" "$junitOutputPath")
fi

BATS_LIB_PATH="$DIR/bats/test_helper" bats "${extraArgs[@]}" "${test_files[@]}"
exitCode=$?

# If we're running in CI, move the test-results to the path that gets
# uploaded. See shell/test.sh.
if in_ci_environment; then
  mkdir -p /tmp/test-results
  mv "$junitOutputPath/"*.xml /tmp/test-results
fi

# Exit with the exit code of the bats tests.
exit $exitCode
