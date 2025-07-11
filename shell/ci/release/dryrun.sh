#!/usr/bin/env bash
# This script attempts to dry-run a release in CI to fake semantic-release.
set -e

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
LIB_DIR="${DIR}/../../lib"

# shellcheck source=../../lib/github.sh
source "${LIB_DIR}/github.sh"

# Setup git user name / email only in CI
if [[ -n $CI ]]; then
  git config --global user.name "Devbase CI"
  git config --global user.email "devbase@outreach.io"
fi

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
# Merge all of the commit messages from the branch into a single commit message.
COMMIT_MESSAGE="$(git log "$CIRCLE_BRANCH".."$OLD_CIRCLE_BRANCH" --reverse --format=%B)"
git checkout "$CIRCLE_BRANCH"

# Squash our branch onto the HEAD (default) branch to mimic
# what would happen after merge.
if ! git diff --quiet "$OLD_CIRCLE_BRANCH"; then
  git merge --squash "$OLD_CIRCLE_BRANCH"
  git commit -m "$COMMIT_MESSAGE"
  GH_TOKEN="$(github_token)"
  if [[ -z $GH_TOKEN ]]; then
    echo "Failed to read Github personal access token" >&2
  fi

  GH_TOKEN="$GH_TOKEN" yarn --frozen-lockfile semantic-release --dry-run

  # Handle prereleases for CLIs, pre-conditions for this exist
  # in the script.
  "$DIR/pre-release.sh" --dry-run
else
  echo "No changes to release"
fi
