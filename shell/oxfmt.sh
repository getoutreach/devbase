#!/usr/bin/env bash
# This is a wrapper around mise to run oxfmt.
# Useful for using the correct version of oxfmt
# with your editor.
set -e

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"

# shellcheck source=./lib/mise/stub.sh
source "$DIR/lib/mise/stub.sh"

mise_exec_tool_with_bin aqua:oxc-project/oxc/oxfmt oxfmt "$@"
