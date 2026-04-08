#!/usr/bin/env bats

load rate_limit.sh

bats_load_library "bats-support/load.bash"
bats_load_library "bats-assert/load.bash"

@test "log_github_rate_limit skips gracefully with empty token" {
  run log_github_rate_limit "" "test_phase" "pat_pool"
  assert_success
  assert_output --partial "skipped (no token available)"
}

@test "log_github_rate_limit skips gracefully with missing token" {
  run log_github_rate_limit
  assert_success
  assert_output --partial "skipped (no token available)"
}

@test "classify_github_token identifies classic PAT" {
  run classify_github_token "ghp_abc123"
  assert_success
  assert_output "pat"
}

@test "classify_github_token identifies fine-grained PAT" {
  run classify_github_token "github_pat_abc123"
  assert_success
  assert_output "pat"
}

@test "classify_github_token identifies app installation token" {
  run classify_github_token "ghs_abc123"
  assert_success
  assert_output "ghapp"
}

@test "classify_github_token identifies OAuth token" {
  run classify_github_token "gho_abc123"
  assert_success
  assert_output "oauth"
}

@test "classify_github_token identifies user-to-server token" {
  run classify_github_token "ghu_abc123"
  assert_success
  assert_output "ghapp_user"
}

@test "classify_github_token returns unknown for unrecognized prefix" {
  run classify_github_token "v1.abc123"
  assert_success
  assert_output "unknown"
}

@test "classify_github_token returns unknown for empty token" {
  run classify_github_token ""
  assert_success
  assert_output "unknown"
}
