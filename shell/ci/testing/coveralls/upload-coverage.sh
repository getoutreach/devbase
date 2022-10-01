#!/usr/bin/env bash
# Uploads code coverage to coveralls.io
#
# Note: This is not meant to be called outside of coverage.sh
set -e

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
SHELL_DIR="$DIR/../../.."
LIB_DIR="$SHELL_DIR/lib"

# shellcheck source=../../../lib/bootstrap.sh
source "${LIB_DIR}/bootstrap.sh"

coverage_file="$1"
flag_name="$2"

if [[ -n $COVERALLS_TOKEN && -f $coverage_file ]]; then
  extra_args=()
  if [[ -n $flag_name ]]; then
    extra_args+=("-parallel" "-flagname" "$flag_name")
  fi

  exec "$SHELL_DIR/gobin.sh" "github.com/mattn/goveralls@v$(get_tool_version "goveralls")" \
    -coverprofile="$coverage_file" -service=circle-ci -jobid="$CIRCLE_WORKFLOW_ID" \
    -repotoken="$COVERALLS_TOKEN" "${extra_args[@]}"
fi
