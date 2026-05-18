#!/usr/bin/env bats

bats_load_library "bats-support/load.bash"
bats_load_library "bats-assert/load.bash"

load mise.sh
load test_helper.sh

setup() {
  REPOPATH=$(mktempdir devbase-lib-mise-XXXXXX)
}

teardown() {
  rm -rf "$REPOPATH"
}

# ---------------------------------------------------------------------------
# version_from_toolversions
# ---------------------------------------------------------------------------

@test "version_from_toolversions returns the version for a known tool" {
  printf 'nodejs 20.11.0\ngolang 1.22.0\n' >"$REPOPATH/.tool-versions"
  run version_from_toolversions "$REPOPATH" nodejs
  assert_success
  assert_output "20.11.0"
}

@test "version_from_toolversions returns only the first version when multiple entries exist" {
  printf 'nodejs 20.11.0\nnodejs 18.19.0\ngolang 1.22.0\n' >"$REPOPATH/.tool-versions"
  run version_from_toolversions "$REPOPATH" nodejs
  assert_success
  assert_output "20.11.0"
}

@test "version_from_toolversions fails when tool is not found" {
  printf 'golang 1.22.0\n' >"$REPOPATH/.tool-versions"
  run version_from_toolversions "$REPOPATH" nodejs
  assert_failure
}

@test "version_from_toolversions fails when .tool-versions is empty" {
  touch "$REPOPATH/.tool-versions"
  run version_from_toolversions "$REPOPATH" nodejs
  assert_failure
}

# ---------------------------------------------------------------------------
# version_all_from_toolversions
# ---------------------------------------------------------------------------

@test "version_all_from_toolversions returns the single version for a tool with one entry" {
  printf 'nodejs 20.11.0\ngolang 1.22.0\n' >"$REPOPATH/.tool-versions"
  run version_all_from_toolversions "$REPOPATH" nodejs
  assert_success
  assert_output "20.11.0"
}

@test "version_all_from_toolversions returns all versions when multiple entries exist" {
  printf 'nodejs 20.11.0\nnodejs 18.19.0\ngolang 1.22.0\n' >"$REPOPATH/.tool-versions"
  run version_all_from_toolversions "$REPOPATH" nodejs
  assert_success
  assert_output "$(printf '20.11.0\n18.19.0')"
}

@test "version_all_from_toolversions fails when tool is not found" {
  printf 'golang 1.22.0\n' >"$REPOPATH/.tool-versions"
  run version_all_from_toolversions "$REPOPATH" nodejs
  assert_failure
}

@test "version_all_from_toolversions fails when .tool-versions is empty" {
  touch "$REPOPATH/.tool-versions"
  run version_all_from_toolversions "$REPOPATH" nodejs
  assert_failure
}
