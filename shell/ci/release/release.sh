#!/usr/bin/env bash
# Release some code
set -e

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
LIB_DIR="${DIR}/../../lib"

# shellcheck source=../../lib/github.sh
source "${LIB_DIR}/github.sh"

# shellcheck source=../../lib/logging.sh
source "${LIB_DIR}/logging.sh"

# shellcheck source=../../lib/bootstrap.sh
source "${LIB_DIR}/bootstrap.sh"

# shellcheck source=../../lib/box.sh
source "${LIB_DIR}/box.sh"

# shellcheck source=../../lib/release.sh
source "${LIB_DIR}/release.sh"

release_exec_engine run

# Retrieve the GH_TOKEN
GITHUB_TOKEN="$(github_token)"
if [[ -z $GITHUB_TOKEN ]]; then
  error "Failed to read GitHub token"
fi

run_gh auth setup-git

# Make https://github.com/pvdlg/env-ci/blob/master/services/circleci.js
# think we're not on a PR.
unset CIRCLE_PR_NUMBER
unset CIRCLE_PULL_REQUESTS
unset CIRCLE_PULL_REQUEST
unset CI_PULL_REQUEST
unset CI_PULL_REQUESTS

# Unset NPM_TOKEN to force it to use the configured ~/.npmrc
NPM_TOKEN='' MISE_GITHUB_TOKEN="$GITHUB_TOKEN" GH_TOKEN="$GITHUB_TOKEN" \
  yarn --frozen-lockfile semantic-release || release_failure_notification
