#!/usr/bin/env bash
# This is a wrapper around mise to run tombi.
# Useful for using the correct version of tombi
# with your editor.
set -e

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"

# shellcheck source=./lib/mise/stub.sh
source "$DIR/lib/mise/stub.sh"

mise_exec_tool tombi "$@"
