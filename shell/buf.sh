#!/bin/bash

set -e

# Wrapper around `buf` that invokes it through asdf/mise with the correct version.
#
# This allows consistent invocation of buf across different scripts and tools.

SCRIPTS_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"

# echo "Running buf.sh script" 2>&1

exec "$SCRIPTS_DIR/asdf-exec.sh" buf "$@"
