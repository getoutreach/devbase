#!/usr/bin/env bash
# Shim to execute `mage` via `mise`.

set -euo pipefail

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"

# shellcheck source=./lib/mise/stub.sh
source "$DIR/lib/mise/stub.sh"

mise_exec_tool mage "$@"
