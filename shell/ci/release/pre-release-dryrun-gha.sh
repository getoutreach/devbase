#!/usr/bin/env bash
#
# Pre-release dryrun script for GitHub Actions.
# This script attempts to dry-run a release in CI to fake semantic-release.

set -eo pipefail

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
LIB_DIR="${DIR}/../../lib"

# shellcheck source=./../../lib/bootstrap.sh
source "$DIR/../../lib/bootstrap.sh"
# shellcheck source=../../lib/logging.sh
source "${LIB_DIR}/logging.sh"
# shellcheck source=./../../lib/yaml.sh
source "$DIR/../../lib/yaml.sh"

# Setup git user name / email only in CI
if [[ -n $CI ]]; then
  git config --global user.name "Devbase CI"
  git config --global user.email "devbase@outreach.io"
fi

git remote -v

# Merge all of the commit messages from the branch into a single commit message.
COMMIT_MESSAGE="$(git log "origin/$GITHUB_BASE_REF".."$GITHUB_REF" --reverse --format=%B)"
git checkout "$GITHUB_BASE_REF"

# Squash our branch onto the HEAD (default) branch to mimic
# what would happen after merge.
if ! git diff --quiet -; then
  git merge --squash -
  git commit -m "$COMMIT_MESSAGE"

  GH_TOKEN="$GITHUB_TOKEN" yarn --frozen-lockfile semantic-release --dry-run

  # If we don't have pre-releasing enabled, notify.
  if [[ "$(yaml_get_field ".arguments.releaseOptions.enablePrereleases" "$(get_service_yaml)")" != "true" ]]; then
    echo "releaseOptions.enablePrereleases is not true, skipping unstable release"
    exit 0
  fi
else
  echo "No changes to release"
fi
