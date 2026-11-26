#!/usr/bin/env bash
# Stops the CircleCI E2E job before its tests when every pull request for
# the branch is a draft without the run-e2e label. The job fails, so a
# skipped run never leaves a passing E2E check on the commit. If a pull
# request can't be read, the E2E tests run, so a GitHub error never skips
# them.
set -eo pipefail

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"

# shellcheck source=../../lib/circleci.sh
source "$DIR/../../lib/circleci.sh"
# shellcheck source=../../lib/github.sh
source "$DIR/../../lib/github.sh"
# shellcheck source=../../lib/logging.sh
source "$DIR/../../lib/logging.sh"

# RUN_E2E_LABEL is the pull request label that runs E2E tests on a draft.
RUN_E2E_LABEL="run-e2e"

if ! circleci_is_pr; then
  exit 0
fi

# CircleCI lists every pull request for the branch in CIRCLE_PULL_REQUESTS
# and picks one of them for CIRCLE_PULL_REQUEST.
IFS=',' read -ra prURLs <<<"${CIRCLE_PULL_REQUESTS:-$CIRCLE_PULL_REQUEST}"

if github_prs_are_drafts_without_label "$RUN_E2E_LABEL" "${prURLs[@]}"; then
  fatal "Skipped the E2E tests: ${prURLs[*]} is a draft." \
    "To run them, mark it ready for review or add the $RUN_E2E_LABEL label, then re-run this job."
fi
