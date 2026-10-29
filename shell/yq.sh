#!/usr/bin/env bash
#
# yq: a jq-compatible query engine over YAML (or JSON) input, backed by
# `devbase yq`. This is a one-shot wrapper around yq_wrapper in
# shell/lib/yq.sh, for callers that cannot source a bash function. See
# that file for how the devbase binary is resolved and cached, and for
# the `gojq` fallback when Go is not installed.
#

set -euo pipefail

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"

# shellcheck source=./lib/yq.sh
source "${DIR}/lib/yq.sh"

yq_wrapper "$@"
