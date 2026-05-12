#!/usr/bin/env bats

bats_load_library "bats-support/load.bash"
bats_load_library "bats-assert/load.bash"

load logging.sh
load shell.sh
load metrics.sh

setup() {
  STUB_DIR="$(mktemp -d -t metrics-stubs-XXXXXX)"
  STUB_CALLS_FILE="$STUB_DIR/calls"
  : >"$STUB_CALLS_FILE"
  export STUB_CALLS_FILE
  export PATH="$STUB_DIR:$PATH"
}

teardown() {
  rm -rf "$STUB_DIR"
  unset STUB_CALLS_FILE
}

# stub_command NAME OUTPUT [EXIT_CODE]
#
# Install an executable stub on PATH that records its invocation to
# $STUB_CALLS_FILE, prints OUTPUT to stdout, and exits with EXIT_CODE
# (default 0).
stub_command() {
  local name="$1" output="$2" exitCode="${3:-0}"
  cat >"$STUB_DIR/$name" <<EOF
#!/usr/bin/env bash
echo "$name \$*" >>"\$STUB_CALLS_FILE"
printf '%s' '$output'
exit $exitCode
EOF
  chmod +x "$STUB_DIR/$name"
}

assert_no_stub_calls() {
  run cat "$STUB_CALLS_FILE"
  assert_output ""
}

@test "report_gh_rate_limit_to_datadog fails when tokenType is empty" {
  run report_gh_rate_limit_to_datadog ""
  assert_failure
  assert_output --partial "tokenType is required"
}

@test "report_gh_rate_limit_to_datadog no-ops when not in CI" {
  stub_command gh '{"used":1,"remaining":4999}'
  stub_command gojq ''
  stub_command curl ''

  CI="" run report_gh_rate_limit_to_datadog app
  assert_success
  # gh and curl must not be invoked outside CI.
  assert_no_stub_calls
}

@test "report_gh_rate_limit_to_datadog no-ops when DATADOG_API_KEY is unset" {
  stub_command gh '{"used":1,"remaining":4999}'
  stub_command gojq ''
  stub_command curl ''

  CI=true DATADOG_API_KEY="" run report_gh_rate_limit_to_datadog app
  assert_success
  assert_no_stub_calls
}

@test "report_gh_rate_limit_to_datadog warns and returns 0 when gh returns invalid JSON" {
  # gh emits a CLI error message on stdout (not JSON).
  stub_command gh 'gh: HTTP 401: Bad credentials'
  stub_command curl ''

  CI=true DATADOG_API_KEY="fake-key" run report_gh_rate_limit_to_datadog app
  assert_success
  assert_output --partial "Returned rate limit is not valid JSON"
  # curl must not be invoked when the rate-limit payload is malformed.
  run grep -c "^curl " "$STUB_CALLS_FILE"
  assert_output "0"
}

@test "report_gh_rate_limit_to_datadog warns and returns 0 when rate limit fields are null" {
  # Valid JSON, but .used and .remaining are absent (e.g., schema drift).
  stub_command gh '{}'
  stub_command curl ''

  CI=true DATADOG_API_KEY="fake-key" run report_gh_rate_limit_to_datadog app
  assert_success
  assert_output --partial "Rate limit response missing used/remaining"
  run grep -c "^curl " "$STUB_CALLS_FILE"
  assert_output "0"
}
