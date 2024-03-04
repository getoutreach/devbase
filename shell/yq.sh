#!/usr/bin/env bash
#
# Wrapper around `yq` that allows use of python-yq or gojq.
# You can force using gojq by setting the environment variable YQ_USE_GOJQ=true.
#

set -euo pipefail
DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"

if [[ -z $GOJQ_VERSION ]]; then
  GOJQ_VERSION="$(grep ^gojq: "$DIR"/../versions.yaml | awk '{print $2}')"
fi

if [[ -n $YQ_USE_GOJQ ]]; then
  use_gojq=true
elif command -v yq >/dev/null 2>&1; then
  # Make sure it's the correct yq. The Go yq is not compatible with jq syntax.
  if [[ "$(yq --version)" =~ ^yq.3.* ]]; then
    use_gojq=false
  fi
else
  use_gojq=true
fi

if [[ $use_gojq == "true" ]]; then
  "$DIR"/gobin.sh github.com/itchyny/gojq/cmd/gojq@"$GOJQ_VERSION" --yaml-input "$@"
else
  yq "$@"
fi
