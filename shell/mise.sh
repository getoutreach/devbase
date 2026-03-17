#!/usr/bin/env bash
#
# Wrapper around mise to ensure a GitHub token is available during execution.

set -euo pipefail

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
DEVBASE_LIB_DIR="$DIR/lib"

# shellcheck source=./lib/mise/stub.sh
source "$DEVBASE_LIB_DIR/mise/stub.sh"

ensure_mise_installed 1>&2

misePath="$(find_mise)"
ghToken="$(run_gh auth token)"

GITHUB_TOKEN="$ghToken" wait_for_gh_rate_limit
MISE_GITHUB_TOKEN="$ghToken" exec "$misePath" "$@"
