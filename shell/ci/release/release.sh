#!/usr/bin/env bash
# Release some code
set -e

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
LIB_DIR="${DIR}/../../lib"

# Read the GH_TOKEN from the file
GH_TOKEN="$(cat "$HOME/.outreach/github.token")"
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

ORIGINAL_VERSION=$(git describe --match 'v[0-9]*' --tags --always HEAD)

# Unset NPM_TOKEN to force it to use the configured ~/.npmrc
NPM_TOKEN='' GH_TOKEN=$GH_TOKEN \
  yarn --frozen-lockfile semantic-release || send_failure_notification

NEW_VERSION=$(git describe --match 'v[0-9]*' --tags --always HEAD)

# Determine if we updated by checking the original version from git
# vs the new version (potentially) after we ran semantic-release.
UPDATED=false
if [[ $ORIGINAL_VERSION != "$NEW_VERSION" ]]; then
  UPDATED=true
fi

# If we didn't update, assume we're on a prerelease branch
# and run the unstable-release code.
if [[ $UPDATED == "false" ]]; then
  exec "$DIR/unstable-release.sh"
fi
