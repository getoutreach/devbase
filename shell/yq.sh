#!/usr/bin/env bash
#
# Wrapper around `yq` that allows use of `python-yq` or `gojq`.
# A locally-installed `gojq` is preferred over `python-yq` if available.
# If `gojq` is not installed locally, you can force using a `gobin`-JIT-executed
# version of `gojq` by setting the environment variable `YQ_USE_JIT_GOJQ=true`.
#

set -euo pipefail

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"

# We don't use get_tool_version from ./lib/bootstrap.sh here because it
# uses this script to read versions.yaml, which would cause a
# circular dependency.
JIT_GOJQ_VERSION="${JIT_GOJQ_VERSION:-$(grep ^gojq: "$DIR"/../versions.yaml | awk '{print $2}')}"

use_jit_gojq=false

# YQ_USE_JIT_GOJQ is meant to be set externally, hence the workaround
# for the unbound variable.
if [[ -n ${YQ_USE_JIT_GOJQ:-} ]]; then
  use_jit_gojq=true
elif command -v yq >/dev/null 2>&1; then
  # Make sure it's the correct yq. The Go yq is not compatible with jq syntax.
  if [[ "$(yq --version)" =~ ^yq.3.* ]]; then
    use_jit_gojq=false
  else
    use_jit_gojq=true
  fi
else
  use_jit_gojq=true
fi

if command -v gojq >/dev/null 2>&1; then
  gojq --yaml-input "$@"
elif [[ $use_jit_gojq == "true" ]]; then
  "$DIR"/gobin.sh github.com/itchyny/gojq/cmd/gojq@"$JIT_GOJQ_VERSION" --yaml-input "$@"
else
  yq "$@"
fi
