#!/usr/bin/env bash
# This script attempts to dry-run a release in CI to fake semantic-release.
set -e

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
LIB_DIR="${DIR}/../../lib"

# shellcheck source=../../lib/bootstrap.sh
source "${LIB_DIR}/bootstrap.sh"

# shellcheck source=../../lib/circleci.sh
source "${LIB_DIR}/circleci.sh"

# shellcheck source=../../lib/github.sh
source "${LIB_DIR}/github.sh"

# shellcheck source=../../lib/logging.sh
source "${LIB_DIR}/logging.sh"

# shellcheck source=../../lib/shell.sh
source "${LIB_DIR}/shell.sh"

# shellcheck source=../../lib/version.sh
source "${LIB_DIR}/version.sh"

# shellcheck source=../../lib/release.sh
source "${LIB_DIR}/release.sh"

# fetch_branch <branch>
#
# Fetches <branch> from origin, unshallowing the clone if it is shallow so the
# branch's full history (needed for merge-base/squash) is present.
fetch_branch() {
  local branch="$1"
  git fetch origin "$branch"
  if [[ -f "$(git rev-parse --git-dir)/shallow" ]]; then
    git fetch --unshallow origin "$branch" || true
  fi
}

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
DEFAULT_BRANCH="$(git rev-parse --abbrev-ref origin/HEAD | sed 's/^origin\///')"

# Fetch the default branch (with tags) before resolving the base. Base
# resolution runs `git merge-base --is-ancestor` against the prereleases
# branch, so its history must be present; in a shallow/single-branch CI clone
# it may be absent, which would misclassify a stable promotion.
fetch_branch "$DEFAULT_BRANCH"

# Resolve the branch to preview against. Stable promotions preview against
# the release branch; everything else against the default branch.
CIRCLE_BRANCH="$(resolve_release_base_branch "$(get_repo_directory)" "$OLD_CIRCLE_BRANCH" "$DEFAULT_BRANCH")"

# Export the branch variable to the semantic-release command
export CIRCLE_BRANCH

# Fetch the resolved base branch, unshallowing as above. When it is the stable
# release branch this is a different ref than the default branch fetched above.
fetch_branch "$CIRCLE_BRANCH"

# The base ref must exist on origin before we can check it out. A missing
# stable release branch would otherwise fail with a raw checkout error.
if ! git rev-parse --verify "origin/$CIRCLE_BRANCH" >/dev/null 2>&1; then
  error "stable release branch 'origin/$CIRCLE_BRANCH' not found on origin."
  fatal "Prerelease-enabled repos must have the '$CIRCLE_BRANCH' branch pushed to origin."
fi

# checkout -B creates or resets the local base branch to the fetched remote
# tip (no long flag equivalent), so the preview starts from origin's state.
git checkout -B "$CIRCLE_BRANCH" "origin/$CIRCLE_BRANCH"

# A missing merge-base means the unshallow above did not fetch enough history
# (the clone is likely still shallow), which would produce a truncated squash
# message rather than a clear failure.
if ! git merge-base "$CIRCLE_BRANCH" "$OLD_CIRCLE_BRANCH" >/dev/null 2>&1; then
  error "unable to find merge-base for '$CIRCLE_BRANCH' and '$OLD_CIRCLE_BRANCH'."
  fatal "The clone is likely still shallow (git fetch --unshallow failed)."
fi

# Decide whether the branch has anything to release, then squash and preview.
# The tri-state exit code keeps a merge conflict from being silently treated
# as "no changes".
set +e
release_has_changes "$(get_repo_directory)" "$CIRCLE_BRANCH" "$OLD_CIRCLE_BRANCH"
rc=$?
set -e

case "$rc" in
0)
  COMMIT_MESSAGE="$(release_commit_message "$(get_repo_directory)" "$CIRCLE_BRANCH" "$OLD_CIRCLE_BRANCH")"
  squash_branch "$(get_repo_directory)" "$CIRCLE_BRANCH" "$OLD_CIRCLE_BRANCH" "$COMMIT_MESSAGE"

  GITHUB_TOKEN="$(github_token)"
  if [[ -z $GITHUB_TOKEN ]]; then
    warn "Failed to read GitHub token" >&2
  fi

  run_gh auth setup-git

  MISE_GITHUB_TOKEN="$GITHUB_TOKEN" GH_TOKEN="$GITHUB_TOKEN" \
    yarn --frozen-lockfile semantic-release --dry-run

  # Handle prereleases for CLIs, pre-conditions for this exist
  # in the script.
  "$DIR/pre-release.sh" --dry-run
  ;;
1)
  echo "No changes to release"
  ;;
*)
  # release_has_changes already logged the diagnostic report to stderr.
  exit 1
  ;;
esac
