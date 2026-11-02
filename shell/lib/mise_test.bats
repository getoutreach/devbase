#!/usr/bin/env bats

bats_load_library "bats-support/load.bash"
bats_load_library "bats-assert/load.bash"

load mise.sh
load yq.sh
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

# lock_missing_versions TOML LOCK
#
# Prints each `tool@version` requested in the [tools] section of TOML
# that has no matching entry in LOCK.
lock_missing_versions() {
  local locked
  locked="$(tomlq_wrapper -c '(.tools // {}) | map_values([.[].version])' "$2")"
  # shellcheck disable=SC2016 # $tool, $req and $locked are jq variables
  tomlq_wrapper -r --argjson locked "$locked" '
    (.tools // {}) | to_entries[] | .key as $tool
    | (.value | if type == "object" then .version elif type == "array" then .[] else . end)
    | select(type == "string") as $req
    | select(($locked[$tool] // []) | any(. == $req or startswith($req + ".") or $req == "latest") | not)
    | "\($tool)@\($req)"' "$1"
}

@test "lock_missing_versions reports a requested version missing from the lockfile" {
  printf '%s\n' '[tools]' '"github:getoutreach/devenv" = "1.68.0"' 'kubectl = "1.34.7"' >"$REPO_DIR/mise.test.toml"
  printf '%s\n' '[[tools."github:getoutreach/devenv"]]' 'version = "1.67.0"' '[[tools.kubectl]]' 'version = "1.34.7"' >"$REPO_DIR/mise.test.lock"
  run lock_missing_versions "$REPO_DIR/mise.test.toml" "$REPO_DIR/mise.test.lock"
  assert_success
  assert_output "github:getoutreach/devenv@1.68.0"
}

@test "lock_missing_versions checks inline table and array tool entries" {
  printf '%s\n' '[tools]' 'node = { version = "20.11.0" }' 'gojq = ["0.12.17", "0.12.18"]' 'kubectl = { version = "1.34.7", os = ["linux"] }' >"$REPO_DIR/mise.test.toml"
  printf '%s\n' '[[tools.node]]' 'version = "20.11.0"' '[[tools.gojq]]' 'version = "0.12.17"' >"$REPO_DIR/mise.test.lock"
  run lock_missing_versions "$REPO_DIR/mise.test.toml" "$REPO_DIR/mise.test.lock"
  assert_success
  assert_output "gojq@0.12.18
kubectl@1.34.7"
}

@test "lock_missing_versions accepts fuzzy and latest requests resolved in the lockfile" {
  printf '%s\n' '[tools]' 'node = "20"' 'shfmt = "latest"' >"$REPO_DIR/mise.test.toml"
  printf '%s\n' '[[tools.node]]' 'version = "20.11.0"' '[[tools.shfmt]]' 'version = "3.12.0"' >"$REPO_DIR/mise.test.lock"
  run lock_missing_versions "$REPO_DIR/mise.test.toml" "$REPO_DIR/mise.test.lock"
  assert_success
  assert_output ""
}

@test "mise.devbase.lock pins every version requested in mise.devbase.toml" {
  run lock_missing_versions "$BATS_TEST_DIRNAME/../../mise.devbase.toml" "$BATS_TEST_DIRNAME/../../mise.devbase.lock"
  assert_success
  assert_output ""
}

@test "mise.e2e.lock pins every version requested in mise.e2e.toml" {
  run lock_missing_versions "$BATS_TEST_DIRNAME/../../mise.e2e.toml" "$BATS_TEST_DIRNAME/../../mise.e2e.lock"
  assert_success
  assert_output ""
}
