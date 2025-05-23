#!/usr/bin/env bash
#
# Wrapper around `yq` that allows use of `python-yq` or `gojq`.
# A locally-installed `gojq` is preferred over `python-yq` if available.
#

set -euo pipefail

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
LIB_DIR="${DIR}/lib"

# shellcheck source=./lib/logging.sh
source "${LIB_DIR}/logging.sh"
# shellcheck source=./lib/mise.sh
source "${LIB_DIR}/mise.sh"
# shellcheck source=./lib/shell.sh
source "${LIB_DIR}/shell.sh"

# find_bin looks for a binary in the PATH or in the mise environment.
# If found, returns the path to the binary.
find_bin() {
  local bin_name="$1"
  if command -v "$bin_name" >/dev/null 2>&1; then
    command -v "$bin_name"
  else
    local mise_path
    mise_path="$(find_mise)"
    if "$mise_path" which "$bin_name" >/dev/null 2>&1; then
      "$mise_path" which "$bin_name"
    fi
  fi
}

gojq_path="$(find_bin gojq)"
if [[ -n $gojq_path ]]; then
  "$gojq_path" --yaml-input "$@"
else
  "$(find_bin yq)" "$@"
fi
