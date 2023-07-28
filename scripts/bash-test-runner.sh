#!/usr/bin/env bash
# Runs all files with _test.sh at the end of the filename
set -euo pipefail

# Find all files with _test.sh at the end of the filename
# and run them
mapfile -t test_files < <(find . -name "*_test.sh" | sort | grep -v "./node_modules")

for test_file in "${test_files[@]}"; do
  echo "Running tests in $test_file"
  # shellcheck disable=SC1090
  bash "$test_file" || {
    echo "Tests failed in '$test_file'"
    exit 1
  }
done
