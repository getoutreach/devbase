#!/usr/bin/env bash
# This script attempts to dry-run a release in CI to fake semantic-release.
set -e

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
LIB_DIR="${DIR}/../../lib"

# shellcheck source=../../lib/circleci.sh
source "${LIB_DIR}/circleci.sh"

# shellcheck source=../../lib/github.sh
source "${LIB_DIR}/github.sh"

# shellcheck source=../../lib/logging.sh
source "${LIB_DIR}/logging.sh"

# shellcheck source=../../lib/shell.sh
source "${LIB_DIR}/shell.sh"

if circleci_pr_is_fork; then
  warn "Skipping pre-release (dry run) check, does not run in CircleCI for PR forks"
  exit 0
fi

# Setup git user name / email only in CI
if in_ci_environment; then
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

# Create a temporary file for the commit message
COMMIT_MSG_FILE="$(mktemp /tmp/commit-message.XXXXXX)"
trap 'rm -f $COMMIT_MSG_FILE' EXIT

# Write commit messages directly to file to avoid argument length limits
git log "$CIRCLE_BRANCH".."$OLD_CIRCLE_BRANCH" --reverse --format=%B >"$COMMIT_MSG_FILE"

git checkout "$CIRCLE_BRANCH"

# Squash our branch onto the HEAD (default) branch to mimic
# what would happen after merge.
if ! git diff --quiet "$OLD_CIRCLE_BRANCH"; then
  git merge --squash "$OLD_CIRCLE_BRANCH"

  # Use the file for the commit message
  git commit -F "$COMMIT_MSG_FILE"

  GITHUB_TOKEN="$(github_token)"
  if [[ -z $GITHUB_TOKEN ]]; then
    warn "Failed to read Github personal access token" >&2
  fi

  MISE_GITHUB_TOKEN="$GITHUB_TOKEN" GH_TOKEN="$GITHUB_TOKEN" \
    yarn --frozen-lockfile semantic-release --dry-run

  # Handle prereleases for CLIs, pre-conditions for this exist
  # in the script.
  "$DIR/pre-release.sh" --dry-run
else
  echo "No changes to release"
fi
