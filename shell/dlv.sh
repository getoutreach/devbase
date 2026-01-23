#!/usr/bin/env bash
# This is a wrapper around mise to run dlv (delve).
# Useful for using the correct version of dlv
# with your editor.
set -e

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"

# shellcheck source=./lib/mise/stub.sh
source "$DIR/lib/mise/stub.sh"

mise_exec_tool_with_bin go:github.com/go-delve/delve/cmd/dlv dlv "$@"
