#!/usr/bin/env bash
# Logs GitHub API rate limit status at end of job.
# Intended to be called via CircleCI post-steps or as a standalone script.
#
# Usage (in CircleCI config post-steps):
#   post-steps:
#     - run:
#         name: Log GitHub rate limit
#         command: ./scripts/shell-wrapper.sh circleci/log_rate_limit.sh job_end
#         when: always
set -e

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
LIB_DIR="$DIR/../lib"

# shellcheck source=../lib/rate_limit.sh
source "${LIB_DIR}/rate_limit.sh"

phase="${1:-job_end}"

# Prefer the PAT from ~/.npmrc (the ghaccesstoken PAT pool, most
# likely to hit rate limits). Fall back to GITHUB_TOKEN (GHAPP).
token=""
if [[ -f "$HOME/.npmrc" ]]; then
  token="$(grep -o '//npm.pkg.github.com/:_authToken=.*' "$HOME/.npmrc" | cut -d= -f2 || true)"
fi
if [[ -z $token ]]; then
  token="${GITHUB_TOKEN:-}"
fi

if [[ -z $token ]]; then
  info_sub "rate limit: skipped (no token in environment)"
  exit 0
fi

log_github_rate_limit "$token" "$phase"
