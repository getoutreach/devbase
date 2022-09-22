#!/usr/bin/env bash
# This is a wrapper around gobin.sh to run shfmt.
# Useful for using the correct version of shfmt
# with your editor.
set -e

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"

# shellcheck source=./lib/bootstrap.sh
source "$DIR/lib/bootstrap.sh"

# Always set simplify mode.
args=("-s" "$@")

# Ensure we're using the correct version of shfmt
#
# Why: We're OK with masking the return value
# shellcheck disable=SC2155
export ASDF_DEFAULT_TOOL_VERSIONS_FILENAME="$(get_repo_directory)/.bootstrap/.tool-versions"

exec shfmt "${args[@]}"
