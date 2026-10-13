#!/usr/bin/env bats

bats_load_library "bats-support/load.bash"
bats_load_library "bats-assert/load.bash"

load mise.sh
load shell.sh
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

@test "mise_shim_dir prefers MISE_SHIMS_DIR" {
  MISE_SHIMS_DIR=/custom/shims MISE_DATA_DIR=/data XDG_DATA_HOME=/xdg run mise_shim_dir
  assert_success
  assert_output "/custom/shims"
}

@test "mise_shim_dir uses MISE_DATA_DIR and strips a trailing slash" {
  MISE_SHIMS_DIR='' MISE_DATA_DIR=/data/ XDG_DATA_HOME=/xdg run mise_shim_dir
  assert_success
  assert_output "/data/shims"
}

@test "mise_shim_dir falls back to XDG_DATA_HOME" {
  MISE_SHIMS_DIR='' MISE_DATA_DIR='' XDG_DATA_HOME=/xdg run mise_shim_dir
  assert_success
  assert_output "/xdg/mise/shims"
}

@test "find_tool skips a mise shim and asks mise" {
  mkdir -p "$REPO_DIR/shims" "$REPO_DIR/bin"
  printf '#!/usr/bin/env bash\n' >"$REPO_DIR/shims/fake-tool"
  chmod +x "$REPO_DIR/shims/fake-tool"
  printf '#!/usr/bin/env bash\necho "$*"\n' >"$REPO_DIR/bin/mise"
  chmod +x "$REPO_DIR/bin/mise"

  MISE_SHIMS_DIR="$REPO_DIR/shims" PATH="$REPO_DIR/shims:$REPO_DIR/bin:$PATH" run find_tool fake-tool
  assert_success
  assert_output "which fake-tool"
}

@test "find_tool returns a non-shim PATH hit directly" {
  mkdir -p "$REPO_DIR/bin"
  printf '#!/usr/bin/env bash\n' >"$REPO_DIR/bin/fake-tool"
  chmod +x "$REPO_DIR/bin/fake-tool"

  MISE_SHIMS_DIR="$REPO_DIR/shims" PATH="$REPO_DIR/bin:$PATH" run find_tool fake-tool
  assert_success
  assert_output "$REPO_DIR/bin/fake-tool"
}
