#!/usr/bin/env bats

bats_load_library "bats-support/load.bash"
bats_load_library "bats-assert/load.bash"

load version.sh

@test "parses v2.5.0" {
  run parse_version "v2.5.0"
  assert_success
  assert_output "2 5 0"
}

@test "parses 2.10.3 (double-digit minor)" {
  run parse_version "2.10.3"
  assert_success
  assert_output "2 10 3"
}

@test "parses major only (v1 -> 1 0 0)" {
  run parse_version "v1"
  assert_success
  assert_output "1 0 0"
}

@test "parses major.minor (3.4 -> 3 4 0)" {
  run parse_version "3.4"
  assert_success
  assert_output "3 4 0"
}

@test "strips pre-release/build metadata" {
  run parse_version "v2.5.0-alpha+001"
  assert_success
  assert_output "2 5 0"
}

@test "handles leading zeros" {
  run parse_version "v02.05.007"
  assert_success
  assert_output "2 5 7"
}

@test "empty input yields zeros" {
  run parse_version ""
  assert_success
  assert_output "0 0 0"
}
