#!/usr/bin/env bash
# Runs all files with .bats at the end of the filename
set -euo pipefail

# DIR is the directory of this script.
DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"

# CI determines if we're running in CI or not. Defaults to false.
CI=${CI:-false}

# Check if bats is installed and usable.
if [[ ! -e "$DIR/bats/bats" ]]; then
  echo "Initializing bats submodule(s) ..."
  git submodule update --init --recursive
fi

# shellcheck source=../shell/lib/shell.sh
source "$DIR/../shell/lib/shell.sh"
# shellcheck source=../shell/lib/bootstrap.sh
source "$DIR/../shell/lib/bootstrap.sh"

# Find all files with _test.sh at the end of the filename
# and run them
mapfile -t test_files < <(find_files_with_extensions "bats")

extraArgs=()
if [[ -n $CI ]]; then
  # If we're running in CI, we want to output junit test results.
  junitOutputPath="$(get_repo_directory)/bin/junit-test-results"
  mkdir -p "$junitOutputPath"

  extraArgs+=("--report-formatter" "junit" "--output" "$junitOutputPath")
fi

BATS_LIB_PATH="$DIR/bats/test_helper" "$DIR/bats/bats/bin/bats" "${extraArgs[@]}" "${test_files[@]}"
exitCode=$?

# If we're running in CI, move the test-results to the path that gets
# uploaded. See shell/test.sh.
if [[ -n $CI ]]; then
  mkdir -p /tmp/test-results
  mv "$junitOutputPath/"*.xml /tmp/test-results
fi

# Exit with the exit code of the bats tests.
exit $exitCode
