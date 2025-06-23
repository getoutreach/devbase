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

gojq_path="$(find_tool gojq)"
if [[ -n $gojq_path ]]; then
  "$gojq_path" --yaml-input "$@"
else
  "$(find_tool yq)" "$@"
fi
