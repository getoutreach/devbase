#!/usr/bin/env bash
# This script attempts to dry-run a release in CI to fake semantic-release.
set -e

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"

# Setup git commands
git config --global user.name "Devbase CI"
git config --global user.email "devbase@outreach.io"

# Make https://github.com/pvdlg/env-ci/blob/master/services/circleci.js
# think we're not on a PR.
unset CIRCLE_PR_NUMBER
unset CIRCLE_PULL_REQUESTS
unset CIRCLE_PULL_REQUEST
unset CI_PULL_REQUEST
unset CI_PULL_REQUESTS

# Store what branch we are really on
OLD_CIRCLE_BRANCH="$CIRCLE_BRANCH"
CIRCLE_BRANCH="$(git rev-parse --abbrev-ref origin/HEAD | sed 's/^origin\///')"

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
git commit -m "$COMMIT_MESSAGE"

GH_TOKEN="$(cat "$HOME/.outreach/github.token")"
if [[ -z $GH_TOKEN ]]; then
  echo "Failed to read Github personal access token" >&2
fi

GH_TOKEN="$GH_TOKEN" yarn --frozen-lockfile semantic-release --dry-run

# Handle unstable releasing for CLIs, pre-conditions for this exist
# in the script.
"$DIR/unstable-release.sh" --dry-run
