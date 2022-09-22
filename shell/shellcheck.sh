#!/usr/bin/env bash
# This is a wrapper around gobin.sh to run shellcheck.
# Useful for using the correct version of shellcheck
# with your editor.
set -e

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"

# shellcheck source=./lib/bootstrap.sh
source "$DIR/lib/bootstrap.sh"

# Always set the correct script directory.
args=("-P" "SCRIPTDIR" "-x" "$@")

# Ensure we're using the correct version of shellcheck
#
# Why: We're OK with masking the return value
# shellcheck disable=SC2155
export ASDF_DEFAULT_TOOL_VERSIONS_FILENAME="$(get_repo_directory)/.bootstrap/.tool-versions"

exec shellcheck "${args[@]}"
