#!/usr/bin/env bash
# Logs GitHub API rate limit status for observability in CI.
#
# Requires the following libraries:
# * logging.sh
# * github.sh (for run_gh)
#
# Usage:
#   source rate_limit.sh
#   read -r token source <<<"$(resolve_github_token)"
#   log_github_rate_limit "$token" "setup_end" "$source"

# LIB_DIR is the directory that shell script libraries live in.
LIB_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"

# shellcheck source=logging.sh
source "$LIB_DIR/logging.sh"

# shellcheck source=github.sh
source "$LIB_DIR/github.sh"

# resolve_github_token prints "token token_source" for rate-limit
# checking. Prefers the PAT from ~/.npmrc (the ghaccesstoken PAT
# pool, most likely to hit rate limits); falls back to the token
# configured in `gh auth` (typically a GitHub App installation token
# set up by bootstrap_github_token).
#
# Callers read the result with:
#   read -r token token_source <<<"$(resolve_github_token)"
resolve_github_token() {
  local token="" source=""
  if [[ -f "$HOME/.npmrc" ]]; then
    token="$(grep -o '//npm.pkg.github.com/:_authToken=.*' "$HOME/.npmrc" | cut -d= -f2 || true)"
    [[ -n $token ]] && source="pat_pool"
  fi
  if [[ -z $token ]]; then
    # github_token() wraps `gh auth token`, which returns whatever
    # token gh is currently authenticated with (typically the GHAPP
    # installation token from bootstrap_github_token).
    set +e
    token="$(github_token 2>/dev/null)"
    set -e
    [[ -n $token ]] && source="ghapp"
  fi
  echo "$token $source"
}

# log_github_rate_limit queries the GitHub rate limit API and logs
# the result as a structured line for easy aggregation.
#
# $1: GitHub token to check
# $2: phase label (e.g., "setup_end", "job_end")
# $3: token source label — identifies the token type for downstream analysis.
#     Use "pat_pool" for ghaccesstoken PATs (5,000 req/hr limit),
#     "ghapp" for GitHub App installation tokens (15,000 req/hr on
#     Enterprise Cloud), or "npm_credentials" for the static PAT from
#     the npm-credentials CircleCI context.
#
# Output format (single line, pipe-delimited for grep):
#   GITHUB_RATE_LIMIT|<phase>|token_source:<source>|core:<used>/<limit>|remaining:<N>|resets_in:<seconds>s|pct_used:<N>%|repo:<CIRCLE_PROJECT_REPONAME>|job:<CIRCLE_JOB>
#
# This function never causes the calling script to exit on failure.
# It is purely observational.
log_github_rate_limit() {
  local token="${1:-}"
  local phase="${2:-unknown}"
  local token_source="${3:-unknown}"

  if [[ -z $token ]]; then
    info_sub "rate limit: skipped (no token available)"
    return 0
  fi

  # GH_TOKEN overrides the active gh auth so we check the specific PAT.
  # GET /rate_limit does not count against the rate limit.
  set +e
  local fields
  fields="$(GH_TOKEN="$token" run_gh api /rate_limit \
    --jq '.resources.core | "\(.limit) \(.used) \(.remaining) \(.reset)"' 2>/dev/null)"
  local gh_exit=$?
  set -e

  if [[ $gh_exit -ne 0 || -z $fields ]]; then
    info_sub "rate limit: skipped (gh api failed)"
    return 0
  fi

  local limit used remaining reset_epoch
  read -r limit used remaining reset_epoch <<<"$fields"

  if [[ -z $limit || -z $remaining ]]; then
    info_sub "rate limit: skipped (parse error)"
    return 0
  fi

  local now resets_in
  now="$(date +%s)"
  resets_in=$((reset_epoch - now))
  if [[ $resets_in -lt 0 ]]; then
    resets_in=0
  fi

  local pct_used=0
  if [[ $limit -gt 0 ]]; then
    pct_used=$(((used * 100) / limit))
  fi

  local repo="${CIRCLE_PROJECT_REPONAME:-unknown}"
  local job="${CIRCLE_JOB:-unknown}"
  echo "GITHUB_RATE_LIMIT|${phase}|token_source:${token_source}|core:${used}/${limit}|remaining:${remaining}|resets_in:${resets_in}s|pct_used:${pct_used}%|repo:${repo}|job:${job}"

  if [[ $remaining -lt 500 ]]; then
    warn "GitHub rate limit: ${used}/${limit} used (${pct_used}%), ${remaining} remaining, resets in ${resets_in}s"
  else
    info_sub "GitHub rate limit: ${used}/${limit} used (${pct_used}%), ${remaining} remaining, resets in ${resets_in}s"
  fi
}
