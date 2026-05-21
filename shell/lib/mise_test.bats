#!/usr/bin/env bats

bats_load_library "bats-support/load.bash"
bats_load_library "bats-assert/load.bash"

load logging.sh
load shell.sh
load mise.sh
load test_helper.sh

setup() {
  REPO_DIR="$(mktempdir devbase-mise-test-XXXXXX)"
}

teardown() {
  rm -rf "$REPO_DIR"
}

# write_tool_versions [entry...]
#
# Write a .tool-versions file in $REPO_DIR with one or more entries.
write_tool_versions() {
  printf '%s\n' "$@" >"$REPO_DIR/.tool-versions"
}

# --- version_from_toolversions ---

@test "version_from_toolversions returns the version for a single-entry tool" {
  write_tool_versions "nodejs 20.11.0"
  run version_from_toolversions "$REPO_DIR" nodejs
  assert_success
  assert_output "20.11.0"
}

@test "version_from_toolversions returns the first version when multiple entries exist for the same tool" {
  write_tool_versions "nodejs 20.11.0" "nodejs 18.19.0"
  run version_from_toolversions "$REPO_DIR" nodejs
  assert_success
  assert_output "20.11.0"
}

@test "version_from_toolversions returns failure when tool is not found" {
  write_tool_versions "golang 1.22.0"
  run version_from_toolversions "$REPO_DIR" nodejs
  assert_failure
  assert_output ""
}

@test "version_from_toolversions returns failure when .tool-versions is empty" {
  write_tool_versions
  run version_from_toolversions "$REPO_DIR" nodejs
  assert_failure
  assert_output ""
}

@test "version_from_toolversions returns the correct tool when multiple different tools are present" {
  write_tool_versions "golang 1.22.0" "nodejs 20.11.0" "python 3.12.0"
  run version_from_toolversions "$REPO_DIR" nodejs
  assert_success
  assert_output "20.11.0"
}

@test "version_from_toolversions does not match a tool that is a prefix of another" {
  write_tool_versions "nodejsextra 16.0.0" "nodejs 20.11.0"
  run version_from_toolversions "$REPO_DIR" nodejs
  assert_success
  assert_output "20.11.0"
}
