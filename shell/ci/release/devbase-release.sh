#!/usr/bin/env bash
#
# Runs a `devbase release` subcommand in CI with the GitHub token of
# the gh CLI. The release scripts call it when releaseOptions.engine in
# service.yaml is "devbase".
set -euo pipefail

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
LIB_DIR="${DIR}/../../lib"

# shellcheck source=../../lib/devbase.sh
source "${LIB_DIR}/devbase.sh"
# shellcheck source=../../lib/github.sh
source "${LIB_DIR}/github.sh"
# shellcheck source=../../lib/logging.sh
source "${LIB_DIR}/logging.sh"
# shellcheck source=../../lib/release.sh
source "${LIB_DIR}/release.sh"

GH_TOKEN="$(github_token)" || true
if [[ -z $GH_TOKEN ]]; then
  warn "Failed to read GitHub token"
fi
# The exec hooks get the token as GH_TOKEN and MISE_GITHUB_TOKEN, as in
# release.sh.
export GH_TOKEN MISE_GITHUB_TOKEN="$GH_TOKEN"

# NPM_TOKEN is empty, so that npm uses the configured ~/.npmrc, as in
# release.sh. devbase_cli replaces the shell for a tagged devbase, so it
# runs in a subshell. A failed publish or run sends the failure notice
# when RELEASE_FAILURE_SLACK_CHANNEL is set.
if ! (NPM_TOKEN='' devbase_cli release "$@"); then
  case "$1" in
  publish | run) release_failure_notification ;;
  *) exit 1 ;;
  esac
fi
