#!/usr/bin/env bash
# Uploads code coverage to coveralls.io
#
# Note: This is not meant to be called outside of coverage.sh
set -euo pipefail

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
LIB_DIR="$DIR/../../../lib"

# shellcheck source=../../../lib/mise/stub.sh
source "${LIB_DIR}/mise/stub.sh"

coverage_file="$1"
flag_name="$2"

if [[ -n ${COVERALLS_TOKEN:-} && -f $coverage_file ]]; then
  extra_args=()
  if [[ -n $flag_name ]]; then
    extra_args+=("-parallel" "-flagname" "$flag_name")
  fi

  mise_exec_tool_with_bin "go:github.com/mattn/goveralls" goveralls \
    -coverprofile="$coverage_file" -service=circle-ci -jobid="${CIRCLE_WORKFLOW_ID:-}" \
    -repotoken="${COVERALLS_TOKEN:-}" "${extra_args[@]}"
fi
