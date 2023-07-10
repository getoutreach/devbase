#!/usr/bin/env bash
# This is a wrapper around asdf to run shfmt.
# Useful for using the correct version of shfmt
# with your editor.
set -e

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"

# shellcheck source=./lib/asdf.sh
source "$DIR/lib/asdf.sh"

# Always set simplify mode.
args=("-s" "$@")

asdf_devbase_exec shfmt "${args[@]}"
