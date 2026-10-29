#!/usr/bin/env bash

bats_load_library "bats-support/load.bash"
bats_load_library "bats-assert/load.bash"

load devbase.sh
load test_helper.sh

# TestDevbaseCliVersion_LocalWhenRunFromDevbaseItself confirms
# devbase_cli_version reports "local" when run from this repository
# (devbase's own), without ever invoking $YQ (shell/yq.sh) -- see
# devbase_tree_root's own comment on why that would recurse.
@test "devbase_cli_version is local when run from devbase's own repo" {
  run devbase_cli_version
  assert_success
  assert_output "local"
}

@test "devbase_tree_root is this repository's own root" {
  run devbase_tree_root
  assert_success
  assert_output "$(cd "$BATS_TEST_DIRNAME/../.." && pwd)"
}

@test "devbase_cli_version reads .version from a fake .bootstrap-style tree" {
  fake_tree="$(mktempdir devbase-cli-test-XXXXXX)"
  echo -n "v2.40.0" >"$fake_tree/.version"

  # Simulate running from inside a consumer repo's .bootstrap by
  # overriding devbase_tree_root's own notion of where devbase
  # lives, and get_repo_directory so it doesn't match tree_root.
  devbase_tree_root() { echo "$fake_tree"; }
  get_repo_directory() { echo "/some/other/consumer/repo"; }

  run devbase_cli_version
  assert_success
  assert_output "v2.40.0"

  rm -rf "$fake_tree"
}

@test "devbase_cli_is_tagged_version accepts release and rc tags only" {
  run devbase_cli_is_tagged_version "v2.40.0"
  assert_success
  run devbase_cli_is_tagged_version "v2.41.0-rc.1"
  assert_success
  run devbase_cli_is_tagged_version "local"
  assert_failure
}

@test "devbase_mise_tool uses the http backend with a checksums.txt checksum_url" {
  run devbase_mise_tool
  assert_success
  assert_output --partial "http:devbase[url=https://github.com/getoutreach/devbase/releases/download/v{{ version }}/"
  assert_output --partial "checksum_url=https://github.com/getoutreach/devbase/releases/download/v{{ version }}/checksums.txt]"
}
