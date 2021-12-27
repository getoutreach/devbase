#!/usr/bin/env bash
# Finish uploading code coverage to coveralls.io
DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
LIB_DIR="${DIR}/../../lib"

# shellcheck source=../../lib/bootstrap.sh
source "${LIB_DIR}/bootstrap.sh"

if [[ -n $COVERALLS_TOKEN ]]; then
  curl "https://coveralls.io/webhook?repo_token=$COVERALLS_TOKEN" -d "payload[build_num]=$CIRCLE_WORKFLOW_ID&payload[status]=done"
fi
