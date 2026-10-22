#!/usr/bin/env bash
#
# yq: a jq-compatible query engine over YAML (or JSON) input, backed by
# `devbase yq` -- see shell/cli.sh for how the devbase CLI itself is
# resolved (a tagged release binary, or `go run` from source).
#
# Some devbase jobs do not have Go installed. In that case, fall back to
# `gojq`, which `devbase yq` replaced. Callers of this script do not use
# the `devbase yq` flags that `gojq` lacks.
#

set -euo pipefail

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"

# shellcheck source=./cli.sh
source "${DIR}/cli.sh"

if ! find_tool go >/dev/null 2>&1; then
  gojq_path="$(find_tool gojq || true)"
  if [[ -z $gojq_path ]]; then
    fatal "neither go nor gojq found; cannot run yq"
  fi
  exec "$gojq_path" --yaml-input "$@"
fi

devbase_cli yq "$@"
