#!/usr/bin/env bash
#
# Wrapper around `yq` that allows use of `python-yq` or `gojq`.
# A locally-installed `gojq` is preferred over `python-yq` if available.
#

set -euo pipefail

if command -v gojq >/dev/null 2>&1; then
  gojq --yaml-input "$@"
else
  yq "$@"
fi
