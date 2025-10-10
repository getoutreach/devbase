#!/usr/bin/env bash
# Release some code
set -e

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
LIB_DIR="${DIR}/../../lib"

# shellcheck source=../../lib/github.sh
source "${LIB_DIR}/github.sh"

# shellcheck source=../../lib/logging.sh
source "${LIB_DIR}/logging.sh"

# Retrieve the GH_TOKEN
GH_TOKEN="$(github_token)"
if [[ -z $GH_TOKEN ]]; then
  error "Failed to read GitHub personal access token"
fi

send_failure_notification() {
  if [[ -z $RELEASE_FAILURE_SLACK_CHANNEL ]]; then
    fatal "Failed to release"
  fi

  curl -X POST "$RELEASE_FAILURE_WEBHOOK" \
    -H 'Content-Type: application/json' \
    -d '{"slackChannel": "'"$RELEASE_FAILURE_SLACK_CHANNEL"'", "buildURL": "'"$CIRCLE_BUILD_URL"'", "repoName": "'"$CIRCLE_PROJECT_REPONAME"'"}'
  exit 1
}

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
NPM_TOKEN='' MISE_GITHUB_TOKEN="$GH_TOKEN" GH_TOKEN=$GH_TOKEN \
  yarn --frozen-lockfile semantic-release || send_failure_notification
