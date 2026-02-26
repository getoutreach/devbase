#!/usr/bin/env bash
# Wrapper for running bats in the context of devbase.

set -euo pipefail

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
DEVBASE_LIB_DIR="$DIR/../shell/lib"

# shellcheck source=../shell/lib/mise/stub.sh
source "$DEVBASE_LIB_DIR/mise/stub.sh"

BATS_LIB_PATH="$DIR/bats/test_helper" mise_exec_tool bats "$@"
