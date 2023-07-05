#!/usr/bin/env bash
# This is a wrapper around asdf to run shellcheck.
# Useful for using the correct version of shellcheck
# with your editor.
set -e

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"

# shellcheck source=./lib/asdf.sh
source "$DIR/lib/asdf.sh"

# Always set the correct script directory.
args=("-P" "SCRIPTDIR" "-x" "$@")
asdf_devbase_exec shellcheck "${args[@]}"
