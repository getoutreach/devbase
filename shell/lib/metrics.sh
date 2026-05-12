#!/usr/bin/env bash
#
# Helpers for emitting CI telemetry to Datadog. Assumes logging.sh
# and shell.sh are sourced.

# report_gh_rate_limit_to_datadog TOKEN_TYPE [EXTRA_TAG...]
#
# Best-effort submission of the current GitHub API rate-limit state
# (for whatever token is in $GITHUB_TOKEN) as Datadog gauge metrics
# under `devbase.github.<token_type>.rate_limit_{used,remaining}`. Always
# tagged with `repo` and `ci_job`; any additional `key:value` tags
# passed as arguments are appended. Silently no-ops when not in CI,
# when DATADOG_API_KEY is unset, when `gh` or `gojq` are unavailable,
# or when the rate-limit query fails. Never returns a non-zero exit
# code.
report_gh_rate_limit_to_datadog() {
  local ddPayload tokenType now rateLimit remaining tags used
  tokenType="${1:-}"
  if [[ -z $tokenType ]]; then
    fatal "report_gh_rate_limit_to_datadog: tokenType is required"
  fi
  shift

  if ! in_ci_environment || [[ -z ${DATADOG_API_KEY:-} ]] || ! command_exists gh || ! command_exists gojq; then
    return 0
  fi

  rateLimit="$(gh api /rate_limit --jq .rate 2>/dev/null || true)"
  if [[ -z $rateLimit ]]; then
    warn "Could not get rate limit from GitHub API, skipping" >&2
    return 0
  fi
  # Validate JSON before feeding to --argjson; a malformed response would
  # otherwise make gojq exit non-zero and, under `set -e` in callers, kill
  # the parent script.
  if ! gojq --exit-status . <<<"$rateLimit" >/dev/null 2>&1; then
    warn "Returned rate limit is not valid JSON, skipping" >&2
    return 0
  fi

  # Why: jq vars, not shell vars
  # shellcheck disable=SC2016
  used="$(gojq --null-input --argjson r "$rateLimit" '$r.used')"
  # Why: jq vars, not shell vars
  # shellcheck disable=SC2016
  remaining="$(gojq --null-input --argjson r "$rateLimit" '$r.remaining')"
  # Guard against `gh api /rate_limit --jq .rate` returning a payload
  # where `.used` or `.remaining` is missing/null (e.g., on auth errors
  # or schema drift). Datadog rejects null numeric values and `curl`'s
  # error is swallowed below, so the metric would silently drop.
  if [[ $used == "null" || $remaining == "null" ]]; then
    warn "Rate limit response missing used/remaining, skipping" >&2
    return 0
  fi
  now="$(date +%s)"
  # Why: jq vars, not shell vars
  # shellcheck disable=SC2016
  tags="$(gojq --null-input --compact-output \
    --arg r "${CIRCLE_PROJECT_REPONAME:-unknown}" \
    --arg j "${CIRCLE_JOB:-unknown}" \
    --args \
    '["repo:" + $r, "ci_job:" + $j] + $ARGS.positional' \
    -- "$@")"
  # type=3 is gauge
  ddPayload=$(
    cat <<EOF
{
  "series": [
    {
      "metric": "devbase.github.${tokenType}.rate_limit_used",
      "type": 3,
      "points": [{"timestamp": $now, "value": $used}],
      "tags": $tags
    },
    {
      "metric": "devbase.github.${tokenType}.rate_limit_remaining",
      "type": 3,
      "points": [{"timestamp": $now, "value": $remaining}],
      "tags": $tags
    }
  ]
}
EOF
  )
  curl --silent --show-error --request POST "https://api.datadoghq.com/api/v2/series" \
    --header "Content-Type: application/json" \
    --header "DD-API-KEY: $DATADOG_API_KEY" \
    --data "$ddPayload" >/dev/null || true
}
