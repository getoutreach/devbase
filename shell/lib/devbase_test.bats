#!/usr/bin/env bash

bats_require_minimum_version 1.5.0

bats_load_library "bats-support/load.bash"
bats_load_library "bats-assert/load.bash"

load logging.sh
load shell.sh
load devbase.sh
load test_helper.sh

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

  # Simulate running from a consumer repo's .bootstrap by stubbing both
  # functions so that the tree root and the repo root differ.
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

@test "devbase_mise_tool uses the http backend with a version-templated release URL" {
  run devbase_mise_tool
  assert_success
  assert_output --partial "http:devbase[url=https://github.com/getoutreach/devbase/releases/download/v{{ version }}/devbase_{{ version }}_"
  refute_output --partial "checksum"
}

@test "devbase_cli_binary uses an exact version match" {
  fake_bin_dir="$(mktempdir devbase-cli-fakebin-XXXXXX)"
  cat >"$fake_bin_dir/devbase" <<'EOF'
#!/usr/bin/env bash
echo "devbase version v2.41.0"
EOF
  chmod +x "$fake_bin_dir/devbase"
  # shellcheck disable=SC2329 # Why: called indirectly as a stub.
  find_tool() { echo "$fake_bin_dir/devbase"; }

  run devbase_cli_binary "v2.41.0"
  assert_success
  assert_output "$fake_bin_dir/devbase"

  rm -rf "$fake_bin_dir"
}

@test "devbase_cli_binary rejects a substring-only version match" {
  fake_bin_dir="$(mktempdir devbase-cli-fakebin-XXXXXX)"
  cat >"$fake_bin_dir/devbase" <<'EOF'
#!/usr/bin/env bash
echo "devbase version v2.41.0-rc.1"
EOF
  chmod +x "$fake_bin_dir/devbase"
  # shellcheck disable=SC2329 # Why: called indirectly as a stub.
  find_tool() { echo "$fake_bin_dir/devbase"; }
  # shellcheck disable=SC2329 # Why: called indirectly as a stub.
  devbase_cli_download_release() { echo "downloaded-instead"; }

  run devbase_cli_binary "v2.41.0"
  assert_success
  assert_output "downloaded-instead"

  rm -rf "$fake_bin_dir"
}

# Regression test: `go run -C` also changed the binary's working
# directory, which broke relative path arguments.
@test "devbase_cli runs the built binary from the caller's own working directory" {
  workdir="$(mktempdir devbase-cli-cwd-XXXXXX)"
  cat >"$workdir/marker.yaml" <<EOF
marker: ok
EOF

  oldpwd="$PWD"
  cd "$workdir" || exit 1
  run devbase_cli yq -r ".marker" marker.yaml
  cd "$oldpwd" || exit 1

  assert_success
  assert_output "ok"

  rm -rf "$workdir"
}

@test "devbase_cli_download_release fails when the installed binary is not found" {
  # shellcheck disable=SC2329 # Why: called indirectly as a stub.
  install_tool_with_mise() { :; }
  # shellcheck disable=SC2329 # Why: called indirectly as a stub.
  run_mise() { return 1; }

  run devbase_cli_download_release "v2.41.0"
  assert_failure
  assert_output --partial "devbase 2.41.0 was installed, but its binary was not found"
}

@test "devbase_cli_download_release echoes the binary path from mise" {
  install_tool_with_mise() { :; }
  run_mise() { echo "/fake/mise/devbase"; }

  run devbase_cli_download_release "v2.41.0"
  assert_success
  assert_output "/fake/mise/devbase"
}

@test "devbase_install_from_source fails without go" {
  # shellcheck disable=SC2329 # Why: called indirectly as a stub.
  find_tool() { return 1; }

  run devbase_install_from_source "$BATS_TEST_TMPDIR/bin"
  assert_failure
  assert_output --partial "using dev version of devbase, needs go installed"
}

@test "devbase_install_from_source installs devbase and the yq and tomlq shims" {
  # shellcheck disable=SC2329 # Why: called indirectly as a stub.
  find_tool() { echo "/fake/go"; }
  # shellcheck disable=SC2329 # Why: called indirectly as a stub.
  go() { echo "built" >"$5"; }

  run devbase_install_from_source "$BATS_TEST_TMPDIR/bin"
  assert_success
  assert_equal "$(cat "$BATS_TEST_TMPDIR/bin/devbase")" "built"
  assert [ -x "$BATS_TEST_TMPDIR/bin/yq" ]
  assert [ -x "$BATS_TEST_TMPDIR/bin/tomlq" ]
}

@test "devbase_build_from_source fails with a message when the build fails" {
  # shellcheck disable=SC2329 # Why: called indirectly as a stub.
  go() { return 1; }

  run devbase_build_from_source "$BATS_TEST_TMPDIR/devbase"
  assert_failure
  assert_output --partial "failed to build devbase from"
}

@test "devbase_build_from_source puts the directory of the go found by find_tool on PATH" {
  # shellcheck disable=SC2329 # Why: called indirectly as a stub.
  find_tool() { echo "/fake/mise/go/bin/go"; }
  # shellcheck disable=SC2329 # Why: called indirectly as a stub.
  go() { echo "$PATH" >"$BATS_TEST_TMPDIR/path"; }

  run devbase_build_from_source "$BATS_TEST_TMPDIR/devbase"
  assert_success
  assert_equal "$(cut -d: -f1 "$BATS_TEST_TMPDIR/path")" "/fake/mise/go/bin"
}
