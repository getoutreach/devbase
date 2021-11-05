#!/usr/bin/env bash
# This script attempts to dry-run a release in CI to fake semantic-release.

# Make https://github.com/pvdlg/env-ci/blob/master/services/circleci.js
# think we're not on a PR.

# Store these and set them after
OLD_CIRCLE_PR_NUMBER=$CIRCLE_PR_NUMBER
OLD_CIRCLE_PULL_REQUEST=$CIRCLE_PULL_REQUEST
OLD_CI_PULL_REQUEST=$CI_PULL_REQUEST
OLD_CIRCLE_BRANCH=$CIRCLE_BRANCH

# Fetch the API URL for usage later.
# 19 comes from the length of https://github.com/
# shellcheck disable=SC2155
export CIRCLE_PR_API_URL=$(echo "https://api.github.com/repos/${CIRCLE_PULL_REQUEST:19}" | sed "s/\/pull\//\/pulls\//")

# Remove evidence of us being on a PR.
unset CIRCLE_PR_NUMBER
unset CIRCLE_PULL_REQUEST
unset CI_PULL_REQUEST

# Fetch the base branch from the API, since CircleCI doesn't expose it.
# shellcheck disable=SC2155
export CIRCLE_BRANCH=$(curl -s -H "Authorization: token ${OUTREACH_GITHUB_TOKEN}" "$CIRCLE_PR_API_URL" | jq -r '.base.ref')

# Act like we're on the base branch.
git branch -D "$CIRCLE_BRANCH" || true
git checkout -b "$CIRCLE_BRANCH" || true

# Run the releaser now.
GH_TOKEN=$OUTREACH_GITHUB_TOKEN yarn --frozen-lockfile semantic-release --dry-run

export CIRCLE_PR_NUMBER=$OLD_CIRCLE_PR_NUMBER
export CIRCLE_PULL_REQUEST=$OLD_CIRCLE_PULL_REQUEST
export CI_PULL_REQUEST=$OLD_CI_PULL_REQUEST
export CIRCLE_BRANCH=$OLD_CIRCLE_BRANCH
