#!/usr/bin/env bash
# Runs all files with _test.sh at the end of the filename
set -euo pipefail

# DIR is the directory of this script.
DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"

# Check if bats is installed and usable.
if [[ ! -e "$DIR/bats/bats" ]]; then
  echo "Initializing bats submodule(s) ..."
  git submodule update --init --recursive
fi

# shellcheck source=shell/lib/shell.sh
source "$DIR/../shell/lib/shell.sh"

# Find all files with _test.sh at the end of the filename
# and run them
mapfile -t test_files < <(find_files_with_extensions "bats")

BATS_LIB_PATH="$DIR/bats/test_helper" exec "$DIR/bats/bats/bin/bats" "${test_files[@]}"
