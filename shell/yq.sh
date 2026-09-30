#!/usr/bin/env bash
#
# yq: a jq-compatible query engine over YAML (or JSON) input, backed by
# `devbase yq` -- see shell/cli.sh for how the devbase CLI itself is
# resolved (a tagged release binary, or `go run` from source).
#

set -euo pipefail

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"

# shellcheck source=./cli.sh
source "${DIR}/cli.sh"

devbase_cli yq "$@"
