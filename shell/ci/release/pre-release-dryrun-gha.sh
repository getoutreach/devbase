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
# shellcheck source=../../lib/shell.sh
source "${LIB_DIR}/shell.sh"
# shellcheck source=../../lib/version.sh
source "${LIB_DIR}/version.sh"
# shellcheck source=../../lib/release.sh
source "${LIB_DIR}/release.sh"

# Setup git user name / email only in CI
if in_ci_environment; then
  git config --global user.name "Devbase CI"
  git config --global user.email "devbase@outreach.io"
  mise use --global gojq
  yarn install --frozen-lockfile
fi

git pull origin "$GITHUB_BASE_REF"

pull_ref=refs/remotes/$(echo "$GITHUB_REF" | cut -d/ -f2-)

git checkout "$GITHUB_BASE_REF"

# Decide whether the PR branch has anything to release, then squash and preview.
# The tri-state exit code keeps a merge conflict from being silently treated
# as "no changes".
set +e
release_has_changes "$(get_repo_directory)" "$GITHUB_BASE_REF" "$pull_ref"
rc=$?
set -e

case "$rc" in
  0)
    COMMIT_MESSAGE="$(release_commit_message "$(get_repo_directory)" "$GITHUB_BASE_REF" "$pull_ref")"
    squash_branch "$(get_repo_directory)" "$GITHUB_BASE_REF" "$pull_ref" "$COMMIT_MESSAGE"

    # Set GitHub Actions builtin variables to not trigger the PR message.
    # From: https://github.com/semantic-release/env-ci/blob/master/services/github.js
    GH_TOKEN="$GITHUB_TOKEN" \
      GITHUB_EVENT_NAME="nonexistent" \
      GITHUB_REF="$GITHUB_BASE_REF" \
      yarn --frozen-lockfile semantic-release --dry-run

    # If we don't have pre-releasing enabled, notify.
    if [[ "$(stencil_arg "releaseOptions.enablePrereleases")" != "true" ]]; then
      echo "releaseOptions.enablePrereleases is not true, skipping unstable release"
      exit 0
    fi
    ;;
  1)
    echo "No changes to release"
    ;;
  *)
    # release_has_changes already logged the diagnostic report to stderr.
    exit 1
    ;;
esac
