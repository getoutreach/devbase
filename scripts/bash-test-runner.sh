#!/usr/bin/env bash
# Runs all files with _test.sh at the end of the filename
set -euo pipefail

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"

# shellcheck source=shell/lib/shell.sh
source "$DIR/../shell/lib/shell.sh"

# Find all files with _test.sh at the end of the filename
# and run them
mapfile -t test_files < <(find_files_with_extensions "sh" | grep -E "_test.sh$")

for test_file in "${test_files[@]}"; do
  echo "Running tests in $test_file"
  # shellcheck disable=SC1090
  bash "$test_file" || {
    echo "Tests failed in '$test_file'"
    exit 1
  }
done
