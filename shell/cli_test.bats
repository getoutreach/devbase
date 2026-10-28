#!/usr/bin/env bats

bats_require_minimum_version 1.5.0

bats_load_library "bats-support/load.bash"
bats_load_library "bats-assert/load.bash"

load lib/logging.sh
load cli.sh
load lib/test_helper.sh

@test "devbase_cli_platform reports this machine's os_arch" {
  run devbase_cli_platform
  assert_success
  assert_output "$(uname -s | tr '[:upper:]' '[:lower:]')_$(uname -m | sed -e 's/x86_64/amd64/' -e 's/aarch64/arm64/')"
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

# Regression test: devbase_cli's from-source path builds with its cwd
# changed to devbase_tree_root (to resolve the module), but must run
# the resulting binary from the CALLER's own, original cwd. A caller
# like shell/lib/bootstrap.sh's stencil_version passes stencil.lock as
# a path relative to its own cwd, not devbase_tree_root's; an earlier
# version of this function used `go run -C`, which changes the
# directory of the whole invocation -- including the resulting
# binary's own working directory -- breaking exactly that case.
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
