#!/usr/bin/env bats

bats_load_library "bats-support/load.bash"
bats_load_library "bats-assert/load.bash"

load logging.sh
load shell.sh
load metrics.sh

setup() {
  STUB_DIR="$(mktemp -d -t metrics-stubs-XXXXXX)"
  STUB_CALLS_FILE="$STUB_DIR/calls"
  STUB_OUTPUTS_DIR="$STUB_DIR/outputs"
  STUB_ARGS_DIR="$STUB_DIR/args"
  mkdir -p "$STUB_OUTPUTS_DIR" "$STUB_ARGS_DIR"
  : >"$STUB_CALLS_FILE"
  export STUB_CALLS_FILE STUB_OUTPUTS_DIR STUB_ARGS_DIR
  export PATH="$STUB_DIR:$PATH"
}

teardown() {
  rm -rf "$STUB_DIR"
  unset STUB_CALLS_FILE STUB_OUTPUTS_DIR STUB_ARGS_DIR
}

# stub_command NAME OUTPUT [EXIT_CODE]
#
# Install an executable stub on PATH that records its invocation to
# $STUB_CALLS_FILE (one line: "NAME ARGS..."), records its full argv to
# $STUB_ARGS_DIR/NAME.argv (one arg per line, latest invocation only),
# prints OUTPUT to stdout, and exits with EXIT_CODE (default 0).
stub_command() {
  local name="$1" output="$2" exitCode="${3:-0}"
  printf '%s' "$output" >"$STUB_OUTPUTS_DIR/$name"
  printf '%s' "$exitCode" >"$STUB_OUTPUTS_DIR/$name.exit"
  cat >"$STUB_DIR/$name" <<'EOF'
#!/usr/bin/env bash
name="$(basename "$0")"
echo "$name $*" >>"$STUB_CALLS_FILE"
printf '%s\n' "$@" >"$STUB_ARGS_DIR/$name.argv"
cat "$STUB_OUTPUTS_DIR/$name"
exit "$(cat "$STUB_OUTPUTS_DIR/$name.exit")"
EOF
  chmod +x "$STUB_DIR/$name"
}

assert_stub_not_called() {
  local name="$1"
  # `run` swallows grep's non-zero exit when the count is 0, so we can
  # assert the count directly without an explicit failure-path check.
  run grep -c "^$name " "$STUB_CALLS_FILE"
  assert_output "0"
}

# Extract the argument passed to curl's `--data` flag (the JSON payload).
curl_payload() {
  awk '/^--data$/ { getline; print; exit }' "$STUB_ARGS_DIR/curl.argv"
}

@test "report_gh_rate_limit_to_datadog fails when tokenType is empty" {
  run report_gh_rate_limit_to_datadog ""
  assert_failure
  assert_output --partial "tokenType is required"
}

@test "report_gh_rate_limit_to_datadog no-ops when not in CI" {
  CI="" run report_gh_rate_limit_to_datadog app
  assert_success
}

@test "report_gh_rate_limit_to_datadog no-ops when DATADOG_API_KEY is unset" {
  CI=true DATADOG_API_KEY="" run report_gh_rate_limit_to_datadog app
  assert_success
}

@test "report_gh_rate_limit_to_datadog warns and returns 0 when gh returns invalid JSON" {
  # gh emits a CLI error on stdout (not JSON).
  stub_command gh 'gh: HTTP 401: Bad credentials'
  # Stub gojq so the test doesn't depend on the real binary; mimic the
  # real exit-code semantics for the validation check (`--exit-status`
  # against non-JSON returns non-zero).
  stub_command gojq '' 1
  stub_command curl ''

  CI=true DATADOG_API_KEY="fake-key" run report_gh_rate_limit_to_datadog app
  assert_success
  assert_output --partial "Returned rate limit is not valid JSON"
  assert_stub_not_called curl
}

@test "report_gh_rate_limit_to_datadog warns and returns 0 when rate limit fields are null" {
  # Real gh + real gojq path requires gojq to be installed; the
  # function builds the payload via gojq and emits `empty` when used
  # or remaining is null. Skip if gojq isn't on PATH (e.g., barebones
  # container).
  if ! command -v gojq >/dev/null 2>&1; then
    skip "gojq not installed"
  fi
  # Valid JSON, but .used and .remaining are absent.
  stub_command gh '{}'
  stub_command curl ''

  CI=true DATADOG_API_KEY="fake-key" run report_gh_rate_limit_to_datadog app
  assert_success
  assert_output --partial "Rate limit response missing used/remaining"
  assert_stub_not_called curl
}

@test "report_gh_rate_limit_to_datadog posts a well-formed Datadog payload on success" {
  if ! command -v gojq >/dev/null 2>&1; then
    skip "gojq not installed"
  fi
  stub_command gh '{"used":42,"remaining":4958}'
  stub_command curl ''

  CI=true \
    DATADOG_API_KEY="fake-key" \
    CIRCLE_PROJECT_REPONAME="my-repo" \
    CIRCLE_JOB="my-job" \
    run report_gh_rate_limit_to_datadog pat consumer:test_consumer
  assert_success

  # curl should have been called exactly once.
  run grep -c "^curl " "$STUB_CALLS_FILE"
  assert_output "1"

  # Verify endpoint and headers.
  run cat "$STUB_ARGS_DIR/curl.argv"
  assert_output --partial "https://api.datadoghq.com/api/v2/series"
  assert_output --partial "DD-API-KEY: fake-key"
  assert_output --partial "Content-Type: application/json"

  # Extract the JSON payload passed to --data and verify its shape.
  local payload
  payload="$(curl_payload)"

  run gojq -r '.series | length' <<<"$payload"
  assert_output "2"

  run gojq -r '.series[0].metric' <<<"$payload"
  assert_output "devbase.github.pat.rate_limit_used"
  run gojq -r '.series[1].metric' <<<"$payload"
  assert_output "devbase.github.pat.rate_limit_remaining"

  run gojq -r '.series[0].type' <<<"$payload"
  assert_output "3"

  run gojq -r '.series[0].points[0].value' <<<"$payload"
  assert_output "42"
  run gojq -r '.series[1].points[0].value' <<<"$payload"
  assert_output "4958"

  run gojq -r '.series[0].tags | sort | join(",")' <<<"$payload"
  assert_output "ci_job:my-job,consumer:test_consumer,repo:my-repo"
}

@test "report_gh_rate_limit_to_datadog JSON-encodes tokenType with quotes safely" {
  if ! command -v gojq >/dev/null 2>&1; then
    skip "gojq not installed"
  fi
  stub_command gh '{"used":1,"remaining":2}'
  stub_command curl ''

  # A pathological tokenType containing a double-quote would break a
  # heredoc-built payload; gojq's --arg encoding must handle it.
  CI=true \
    DATADOG_API_KEY="fake-key" \
    run report_gh_rate_limit_to_datadog 'evil"name'
  assert_success

  local payload
  payload="$(curl_payload)"

  # Payload must still be valid JSON.
  run gojq -e . <<<"$payload"
  assert_success
  run gojq -r '.series[0].metric' <<<"$payload"
  assert_output 'devbase.github.evil"name.rate_limit_used'
}
