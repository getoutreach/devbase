#!/usr/bin/env bats

bats_load_library "bats-support/load.bash"
bats_load_library "bats-assert/load.bash"

load logging.sh
load shell.sh
load metrics.sh

@test "report_gh_rate_limit_to_datadog fails when tokenType is empty" {
  run report_gh_rate_limit_to_datadog ""
  assert_failure
  assert_output --partial "tokenType is required"
}
