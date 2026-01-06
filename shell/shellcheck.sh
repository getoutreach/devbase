#!/usr/bin/env bash
# This is a wrapper around mise to run shellcheck.
# Useful for using the correct version of shellcheck
# with your editor.
set -e

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"

# shellcheck source=./lib/mise/stub.sh
source "$DIR/lib/mise/stub.sh"

# Always set the correct script directory.
args=(--external-sources --source-path=SCRIPTDIR "$@")

mise_exec_tool shellcheck "${args[@]}"
