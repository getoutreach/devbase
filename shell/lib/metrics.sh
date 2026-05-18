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
# passed as arguments are appended.
#
# Silently no-ops (returns 0) when not in CI, when DATADOG_API_KEY is
# unset, when `gh` or `gojq` are unavailable, or when the rate-limit
# query fails for any reason (network, malformed JSON, missing fields,
# Datadog rejection).
#
# Exits the calling shell via `fatal` if TOKEN_TYPE is empty -- this is
# a programmer error (a caller passed an unset variable) and should
# fail loudly rather than emit metrics under a meaningless key.
report_gh_rate_limit_to_datadog() {
  local ddPayload tokenType now rateLimit
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

  now="$(date +%s)"
  # Build the entire Datadog payload with gojq so every dynamic value
  # (including `tokenType` and the extra tags) is properly JSON-encoded.
  # type=3 is gauge.
  # shellcheck disable=SC2016 # jq vars, not shell vars
  ddPayload="$(gojq --null-input --compact-output \
    --argjson rateLimit "$rateLimit" \
    --argjson now "$now" \
    --arg tokenType "$tokenType" \
    --arg repo "${CIRCLE_PROJECT_REPONAME:-unknown}" \
    --arg job "${CIRCLE_JOB:-unknown}" \
    --args \
    '
      ($rateLimit.used) as $used |
      ($rateLimit.remaining) as $remaining |
      (["repo:" + $repo, "ci_job:" + $job] + $ARGS.positional) as $tags |
      if ($used == null or $remaining == null) then
        empty
      else
        {
          series: [
            {
              metric: ("devbase.github." + $tokenType + ".rate_limit_used"),
              type: 3,
              points: [{ timestamp: $now, value: $used }],
              tags: $tags
            },
            {
              metric: ("devbase.github." + $tokenType + ".rate_limit_remaining"),
              type: 3,
              points: [{ timestamp: $now, value: $remaining }],
              tags: $tags
            }
          ]
        }
      end
    ' \
    -- "$@")"

  # gojq emitted `empty` -- used or remaining was null. Datadog rejects
  # null numeric values; warn and skip rather than send a malformed
  # request that would be silently swallowed by `curl || true` below.
  if [[ -z $ddPayload ]]; then
    warn "Rate limit response missing used/remaining, skipping" >&2
    return 0
  fi

  curl --silent --show-error --request POST "https://api.datadoghq.com/api/v2/series" \
    --header "Content-Type: application/json" \
    --header "DD-API-KEY: $DATADOG_API_KEY" \
    --data "$ddPayload" >/dev/null || true
}
