#!/usr/bin/env bats

bats_load_library "bats-support/load.bash"
bats_load_library "bats-assert/load.bash"

load version.sh

@test "parse_version parses v2.5.0 (single-digit minor)" {
  run parse_version "v2.5.0"
  assert_success
  assert_output "2 5 0"
}

@test "parse_version parses 2.10.3 (double-digit minor)" {
  run parse_version "v2.10.3"
  assert_success
  assert_output "2 10 3"
}

@test "parse_version parses major only (v1 -> 1 0 0)" {
  run parse_version "v1"
  assert_success
  assert_output "1 0 0"
}

@test "parse_version parses major.minor (3.4 -> 3 4 0)" {
  run parse_version "v3.4"
  assert_success
  assert_output "3 4 0"
}

@test "parse_version strips pre-release/build metadata" {
  run parse_version "v2.5.0-alpha+001"
  assert_success
  assert_output "2 5 0"
}

@test "parse_version handles leading zeros" {
  run parse_version "v02.05.007"
  assert_success
  assert_output "2 5 7"
}

@test "parse_version handles only numeric version (no v prefix)" {
  run parse_version "2.5.0"
  assert_success
  assert_output "2 5 0"
}

@test "parse_version with empty input yields zeros" {
  run parse_version ""
  assert_success
  assert_output "0 0 0"
}

@test "parse_version fails when a version part contains a nonnumeric character" {
  run parse_version "1.0.0a"
  assert_failure
}

@test "has_minimum_version succeeds for equal versions" {
  run has_minimum_version "2.5.0" "v2.5.0"
  assert_success
}

@test "has_minimum_version succeeds when version is greater" {
  run has_minimum_version "2.5.0" "2.6.0"
  assert_success
}

@test "has_minimum_version fails when version is lower" {
  run has_minimum_version "2.5.0" "2.4.9"
  assert_failure
}

@test "has_minimum_version handles leading zeros" {
  run has_minimum_version "2.5.0" "v02.05.007"
  assert_success
}

@test "has_minimum_version treats empty version as 0.0.0 and fails when min > 0" {
  run has_minimum_version "1.0.0" ""
  assert_failure
}

@test "has_minimum_version treats empty minimum as 0.0.0 and succeeds for any positive version" {
  run has_minimum_version "" "v1.2.3"
  assert_success
}

@test "has_minimum_version handles higher minor version" {
  run has_minimum_version "2.5.0" "v1.62.2"
  assert_failure
}
