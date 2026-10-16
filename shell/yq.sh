#!/usr/bin/env bash
#
# Wrapper around `yq` that allows use of `python-yq` or `gojq`.
# A locally-installed `gojq` is preferred over `python-yq` if available.
#
# See lib/yq.sh for why gojq's input is checked against a set of
# known-unsupported python-yq flags before gojq is invoked.
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
# shellcheck source=./lib/yq.sh
source "${LIB_DIR}/yq.sh"

gojq_path="$(find_tool gojq)"
if [[ -n $gojq_path ]]; then
  check_unsupported_yq_flags "$@"
  "$gojq_path" --yaml-input "${NORMALIZED_YQ_ARGS[@]}"
else
  "$(find_tool yq)" "$@"
fi
