#!/usr/bin/env bash
# This script attempts to dry-run a release in CI to fake semantic-release.
set -e

# Make https://github.com/pvdlg/env-ci/blob/master/services/circleci.js
# think we're not on a PR.
unset CIRCLE_PR_NUMBER
unset CIRCLE_PULL_REQUESTS
unset CIRCLE_PULL_REQUEST
unset CI_PULL_REQUEST
unset CI_PULL_REQUESTS

# Store what branch we are really on
OLD_CIRCLE_BRANCH="$CIRCLE_BRANCH"

# Determine what our head branch is, with the default assumption
# being that it's main.
CIRCLE_BRANCH=""
if git rev-parse main >/dev/null 2>&1; then
  CIRCLE_BRANCH="main"
elif git rev-parse master >/dev/null 2>&1; then
  CIRCLE_BRANCH="master"
else
  echo "Error: Failed to determine HEAD (default) branch" >&2
fi

# Export the branch variable to the semantic-release command
export CIRCLE_BRANCH

# Checkout the HEAD (default) branch and ensure it's up-to-date.
git checkout "$CIRCLE_BRANCH"
git pull

git checkout "$OLD_CIRCLE_BRANCH"
# Grab the first commit's message
COMMIT_MESSAGE=$(git log "$CIRCLE_BRANCH".."$OLD_CIRCLE_BRANCH" --oneline | tail -1 | sed -E 's/^[a-zA-Z0-9]+ //')
git checkout "$CIRCLE_BRANCH"

# Squash our branch onto the HEAD (default) branch to mimic
# what would happen after merge.
git merge --squash "$OLD_CIRCLE_BRANCH"
git config --global user.name "Devbase CI"
git config --global user.email "devbase@outreach.io"
git commit -m "$COMMIT_MESSAGE"

GH_TOKEN=$OUTREACH_GITHUB_TOKEN yarn --frozen-lockfile semantic-release --dry-run
