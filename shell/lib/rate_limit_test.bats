#!/usr/bin/env bats

load rate_limit.sh

bats_load_library "bats-support/load.bash"
bats_load_library "bats-assert/load.bash"

@test "log_github_rate_limit skips gracefully with empty token" {
  run log_github_rate_limit "" "test_phase"
  assert_success
  assert_output --partial "skipped (no token available)"
}

@test "log_github_rate_limit skips gracefully with missing token" {
  run log_github_rate_limit
  assert_success
  assert_output --partial "skipped (no token available)"
}
