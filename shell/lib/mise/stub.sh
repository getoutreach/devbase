#!/usr/bin/env bash
#
# Helper for `mise`-based tool stubs.

DIRNAME="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
DEVBASE_LIB_DIR="$DIRNAME/.."

# shellcheck source=../bootstrap.sh
source "$DEVBASE_LIB_DIR/bootstrap.sh"
# shellcheck source=../github.sh
source "$DEVBASE_LIB_DIR/github.sh"
# shellcheck source=../logging.sh
source "$DEVBASE_LIB_DIR/logging.sh"
# shellcheck source=../mise.sh
source "$DEVBASE_LIB_DIR/mise.sh"
# shellcheck source=../shell.sh
source "$DEVBASE_LIB_DIR/shell.sh"
