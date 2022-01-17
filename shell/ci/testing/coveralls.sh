#!/usr/bin/env bash
# Uploads code coverage to coveralls.io
DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
SHELL_DIR="$DIR/../.."
LIB_DIR="${DIR}/../../lib"

# shellcheck source=../../lib/bootstrap.sh
source "${LIB_DIR}/bootstrap.sh"

flag_name="$1"
coverage_file=/tmp/coverage.out

if [[ -n $COVERALLS_TOKEN && -f $coverage_file ]]; then
  extra_args=()
  if [[ -n $flag_name ]]; then
    extra_args+=("-parallel" "-flagname" "$flag_name")
  fi

  "$SHELL_DIR/gobin.sh" "github.com/mattn/goveralls@v$(get_tool_version "goveralls")" -coverprofile="$coverage_file" -service=circle-ci -jobid="$CIRCLE_WORKFLOW_ID" -repotoken="$COVERALLS_TOKEN" "${extra_args[@]}"
fi
