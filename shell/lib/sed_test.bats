#!/usr/bin/env bats

bats_require_minimum_version 1.5.0

bats_load_library "bats-support/load.bash"
bats_load_library "bats-assert/load.bash"

load logging.sh
load sed.sh
load test_helper.sh

setup() {
  testdir="$(mktempdir devbase-sed-test-XXXXXX)"
}

teardown() {
  rm -rf "$testdir"
  unset testdir
  unset MACOS_GNU_SED
}

@test "sed_replace replaces text in a file" {
  echo "hello world" >"$testdir/file.txt"
  sed_replace "world" "universe" "$testdir/file.txt"
  run cat "$testdir/file.txt"
  assert_success
  assert_output "hello universe"
}

@test "sed_replace fails if gsed is not installed on macOS" {
  if ! [[ $OSTYPE =~ darwin* ]]; then
    skip "Skipping macOS-specific test on non-macOS system."
  fi
  export MACOS_GNU_SED="nonexistent-gsed"
  echo "hello world" >"$testdir/file.txt"
  run --separate-stderr sed_replace "world" "universe" "$testdir/file.txt"
  assert_failure
  assert_stderr --partial "macOS support requires GNU sed"
}
