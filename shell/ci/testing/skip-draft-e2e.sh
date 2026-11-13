#!/usr/bin/env bash
# Halts the CircleCI E2E job when it runs for a draft pull request,
# unless the pull request has the run-e2e label. If the pull request
# can't be read, the E2E tests run, so a GitHub error never skips them.
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

if circleci_is_pr && github_pr_is_draft_without_label "$CIRCLE_PULL_REQUEST" "$RUN_E2E_LABEL"; then
  info "Skipping E2E tests on draft pull request $CIRCLE_PULL_REQUEST." \
    "To run them, mark it ready for review or add the $RUN_E2E_LABEL label."
  circleci-agent step halt
fi
