#!/usr/bin/env bash
# Release some code
set -e

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
LIB_DIR="${DIR}/../../lib"

# Retrieve the GH_TOKEN
GH_TOKEN="$(gh auth token)"
if [[ -z $GH_TOKEN ]]; then
  echo "Failed to read Github personal access token" >&2
fi

send_failure_notification() {
  if [[ -z $RELEASE_FAILURE_SLACK_CHANNEL ]]; then
    echo "Failed to release"
    exit 1
  fi

  curl -X POST "$RELEASE_FAILURE_WEBHOOK" \
    -H 'Content-Type: application/json' \
    -d '{"slackChannel": "'"$RELEASE_FAILURE_SLACK_CHANNEL"'", "buildURL": "'"$CIRCLE_BUILD_URL"'", "repoName": "'"$CIRCLE_PROJECT_REPONAME"'"}'
  exit 1
}

# shellcheck source=../../lib/logging.sh
source "${LIB_DIR}/logging.sh"

# shellcheck source=../../lib/bootstrap.sh
source "${LIB_DIR}/bootstrap.sh"

# shellcheck source=../../lib/box.sh
source "${LIB_DIR}/box.sh"

# Make https://github.com/pvdlg/env-ci/blob/master/services/circleci.js
# think we're not on a PR.
unset CIRCLE_PR_NUMBER
unset CIRCLE_PULL_REQUESTS
unset CIRCLE_PULL_REQUEST
unset CI_PULL_REQUEST
unset CI_PULL_REQUESTS

# Unset NPM_TOKEN to force it to use the configured ~/.npmrc
NPM_TOKEN='' GH_TOKEN=$GH_TOKEN \
  yarn --frozen-lockfile semantic-release || send_failure_notification
