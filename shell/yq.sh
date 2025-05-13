#!/usr/bin/env bash
#
# Wrapper around `yq` that allows use of `python-yq` or `gojq`.
# A locally-installed `gojq` is preferred over `python-yq` if available.
#

set -euo pipefail

find_gojq() {
  local
  if command -v gojq >/dev/null 2>&1; then
    command -v gojq
  else
    mise which gojq
  fi
}

gojq_path="$(find_gojq)"
if [[ -n $gojq_path ]]; then
  "$gojq_path" --yaml-input "$@"
else
  yq "$@"
fi
