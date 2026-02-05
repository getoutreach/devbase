#!/usr/bin/env bash
#
# Wrapper around mise + goimports, mostly for editors that can't handle
# `mise exec` in their config.
set -euo pipefail

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"

# shellcheck source=./lib/mise/stub.sh
source "$DIR/lib/mise/stub.sh"

mise_exec_tool_with_bin go:golang.org/x/tools/cmd/goimports goimports "$@"
