#!/usr/bin/env bash
# Uploads code coverage to coveralls.io
DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
SHELL_DIR="$DIR/../.."
LIB_DIR="${DIR}/../../lib"

# shellcheck source=../../lib/bootstrap.sh
source "${LIB_DIR}/bootstrap.sh"

if [[ -n $COVERALLS_TOKEN ]]; then
  "$SHELL_DIR/gobin.sh" "github.com/mattn/goveralls@$(get_tool_version "goveralls")" -coverprofile=/tmp/coverage.out -service=circle-ci -repotoken="$COVERALLS_TOKEN"
fi
