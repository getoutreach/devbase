#!/usr/bin/env bats

bats_require_minimum_version 1.5.0

bats_load_library "bats-support/load.bash"
bats_load_library "bats-assert/load.bash"

load lib/logging.sh
load cli.sh
load lib/test_helper.sh

# TestDevbaseCliVersion_LocalWhenRunFromDevbaseItself confirms
# devbase_cli_version reports "local" when run from this repository
# (devbase's own), without ever invoking $YQ (shell/yq.sh) -- see
# cli.sh's own comment on why that would recurse.
@test "devbase_cli_version is local when run from devbase's own repo" {
  run devbase_cli_version
  assert_success
  assert_output "local"
}

@test "devbase_tree_root is this repository's own root" {
  run devbase_tree_root
  assert_success
  assert_output "$(cd "$BATS_TEST_DIRNAME/.." && pwd)"
}

@test "devbase_cli_platform reports this machine's os_arch" {
  run devbase_cli_platform
  assert_success
  assert_output "$(uname -s | tr '[:upper:]' '[:lower:]')_$(uname -m | sed -e 's/x86_64/amd64/' -e 's/aarch64/arm64/')"
}

@test "devbase_cli_version reads .version from a fake .bootstrap-style tree" {
  fake_tree="$(mktempdir devbase-cli-test-XXXXXX)"
  echo -n "v2.40.0" >"$fake_tree/.version"

  # Simulate running from inside a consumer repo's .bootstrap by
  # overriding DIR/devbase_tree_root's own notion of where this script
  # lives, and get_repo_directory so it doesn't match tree_root.
  devbase_tree_root() { echo "$fake_tree"; }
  get_repo_directory() { echo "/some/other/consumer/repo"; }

  run devbase_cli_version
  assert_success
  assert_output "v2.40.0"

  rm -rf "$fake_tree"
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

# TestDevbaseCliBinary_RejectsSubstringMatch confirms a version match
# is exact, not a substring search: a requested "v2.41.0" must not
# match an installed "v2.41.0-rc.1" just because the latter contains
# the former as a substring (a real devbase tag pattern).
@test "devbase_cli_binary rejects a substring-only version match" {
  fake_bin_dir="$(mktempdir devbase-cli-fakebin-XXXXXX)"
  cat >"$fake_bin_dir/devbase" <<'EOF'
#!/usr/bin/env bash
echo "devbase version v2.41.0-rc.1"
EOF
  chmod +x "$fake_bin_dir/devbase"
  find_tool() { echo "$fake_bin_dir/devbase"; }
  devbase_cli_download_release() { echo "downloaded-instead"; }

  run devbase_cli_binary "v2.41.0"
  assert_success
  assert_output "downloaded-instead"

  rm -rf "$fake_bin_dir"
}
