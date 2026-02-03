#!/usr/bin/env bash
#
# Wrapper around mise to ensure a GitHub token is available during execution.

set -euo pipefail

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
DEVBASE_LIB_DIR="$DIR/lib"

# shellcheck source=./lib/logging.sh
source "$DEVBASE_LIB_DIR/logging.sh"

# shellcheck source=./lib/mise.sh
source "$DEVBASE_LIB_DIR/mise.sh"

# shellcheck source=./lib/shell.sh
source "$DEVBASE_LIB_DIR/shell.sh"

ensure_mise_installed 1>&2

misePath="$(find_mise)"

wait_for_gh_rate_limit
MISE_GITHUB_TOKEN="$(gh auth token)" exec "$misePath" "$@"
