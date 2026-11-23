#!/usr/bin/env bash

bats_require_minimum_version 1.5.0

bats_load_library "bats-support/load.bash"
bats_load_library "bats-assert/load.bash"

load logging.sh
load shell.sh
load devbase.sh
load test_helper.sh

setup() {
  # Isolate the binary cache per test so a stubbed build never collides
  # with a real cached binary.
  DEVBASE_CACHED_BINARY_STORAGE_PATH="$(mktempdir devbase-cache-XXXXXX)"
  export DEVBASE_CACHED_BINARY_STORAGE_PATH
}

teardown() {
  rm -rf "$DEVBASE_CACHED_BINARY_STORAGE_PATH"
  unset DEVBASE_CACHED_BINARY_STORAGE_PATH
}

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
  # shellcheck disable=SC2329 # Why: called indirectly as a stub.
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

# make_git_repo creates $repo with one commit at a fixed time.
make_git_repo() {
  repo="$(mktempdir devbase-version-XXXXXX)"
  export GIT_AUTHOR_NAME=t GIT_AUTHOR_EMAIL=t@example.com GIT_AUTHOR_DATE="2026-03-01T12:00:00Z"
  export GIT_COMMITTER_NAME=t GIT_COMMITTER_EMAIL=t@example.com GIT_COMMITTER_DATE="$GIT_AUTHOR_DATE"
  # Ignore the user's git config: signing settings make `git tag` prompt.
  export GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_SYSTEM=/dev/null
  git -C "$repo" init -q
  git -C "$repo" commit -q --allow-empty -m one
}

@test "devbase_version_from_git fails outside a git checkout" {
  run devbase_version_from_git "$(mktempdir devbase-version-XXXXXX)"
  assert_failure
  assert_output ""
}

@test "devbase_version_from_git uses v0.0.0 when there are no tags" {
  make_git_repo
  run devbase_version_from_git "$repo"
  assert_success
  assert_output --regexp '^v0\.0\.0-20260301120000-[0-9a-f]{12}$'
}

@test "devbase_version_from_git echoes the tag when HEAD is on it" {
  make_git_repo
  git -C "$repo" tag v1.2.3
  run devbase_version_from_git "$repo"
  assert_success
  assert_output "v1.2.3"
}

@test "devbase_version_from_git bumps the patch after a release tag" {
  make_git_repo
  git -C "$repo" tag v1.2.3
  git -C "$repo" commit -q --allow-empty -m two
  run devbase_version_from_git "$repo"
  assert_success
  assert_output --regexp '^v1\.2\.4-0\.20260301120000-[0-9a-f]{12}$'
}

@test "devbase_version_from_git extends a prerelease tag" {
  make_git_repo
  git -C "$repo" tag v1.3.0-rc.1
  git -C "$repo" commit -q --allow-empty -m two
  run devbase_version_from_git "$repo"
  assert_success
  assert_output --regexp '^v1\.3\.0-rc\.1\.0\.20260301120000-[0-9a-f]{12}$'
}

@test "devbase_version_from_git prefers a release over a prerelease of the same version" {
  make_git_repo
  git -C "$repo" tag v1.3.0-rc.1
  git -C "$repo" tag v1.3.0
  git -C "$repo" commit -q --allow-empty -m two
  run devbase_version_from_git "$repo"
  assert_success
  assert_output --regexp '^v1\.3\.1-0\.'
}

@test "devbase_version_from_git marks a dirty tree" {
  make_git_repo
  git -C "$repo" tag v1.2.3
  echo x >"$repo/new"
  run devbase_version_from_git "$repo"
  assert_success
  assert_output "v1.2.3+dirty"
}

@test "devbase_build_from_source sets gobox's app.Version when a version is known" {
  # shellcheck disable=SC2329 # Why: called indirectly as a stub.
  find_tool() { echo "/fake/go"; }
  # shellcheck disable=SC2329 # Why: called indirectly as a stub.
  devbase_version_from_git() { echo "v9.9.9"; }
  # shellcheck disable=SC2329 # Why: called indirectly as a stub.
  go() { echo "$*" >"$BATS_TEST_TMPDIR/args"; }

  run devbase_build_from_source "$BATS_TEST_TMPDIR/devbase"
  assert_success
  assert_output ""
  run cat "$BATS_TEST_TMPDIR/args"
  assert_output --partial "-ldflags -X github.com/getoutreach/gobox/pkg/app.Version=v9.9.9"
}

@test "devbase_build_from_source omits ldflags when no version is known" {
  # shellcheck disable=SC2329 # Why: called indirectly as a stub.
  find_tool() { echo "/fake/go"; }
  # shellcheck disable=SC2329 # Why: called indirectly as a stub.
  devbase_version_from_git() { return 1; }
  # shellcheck disable=SC2329 # Why: called indirectly as a stub.
  go() { echo "$*" >"$BATS_TEST_TMPDIR/args"; }

  run devbase_build_from_source "$BATS_TEST_TMPDIR/devbase"
  assert_success
  run cat "$BATS_TEST_TMPDIR/args"
  refute_output --partial "ldflags"
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

# Stubs a release devbase that logs its arguments and the auth-check setting.
make_logging_devbase() {
  fake_bin="$BATS_TEST_TMPDIR/devbase"
  calls="$BATS_TEST_TMPDIR/calls"
  cat >"$fake_bin" <<EOF
#!/usr/bin/env bash
echo "\$* auth=\${GOBOX_SKIP_VALIDATE_AUTH:-unset}" >>"$calls"
echo "devbase version v2.41.0"
EOF
  chmod +x "$fake_bin"
  # shellcheck disable=SC2329 # Why: called indirectly as a stub.
  devbase_cli_version() { echo "v2.41.0"; }
  # shellcheck disable=SC2329 # Why: called indirectly as a stub.
  find_tool() { echo "$fake_bin"; }
}

@test "devbase_cli never self-updates devbase, and skips the GitHub auth check in CI" {
  make_logging_devbase
  # shellcheck disable=SC2329 # Why: called indirectly as a stub.
  in_ci_environment() { return 0; }

  run devbase_cli yq
  assert_success
  assert_equal "$(cat "$calls")" $'--skip-update --version auth=true\n--skip-update yq auth=true'
}

@test "devbase_cli never self-updates devbase, and leaves the GitHub auth check outside CI" {
  make_logging_devbase
  # shellcheck disable=SC2329 # Why: called indirectly as a stub.
  in_ci_environment() { return 1; }

  run devbase_cli yq
  assert_success
  assert_equal "$(cat "$calls")" $'--skip-update --version auth=unset\n--skip-update yq auth=unset'
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

# stub_go_git stubs `go` and `git` for devbase_cached_source_binary. `git`
# reports the commit $2 (default "deadbeefcafe") and, if $3 is set, a dirty
# tree with that diff. `go build -o <path>` creates an empty executable at
# <path> and counts calls in the file $1.
stub_go_git() {
  local build_count_file="$1"
  local rev="${2:-deadbeefcafe}"
  local dirty_diff="${3:-}"
  [[ -e $build_count_file ]] || echo 0 >"$build_count_file"

  # shellcheck disable=SC2317 # Why: called indirectly as a stub.
  go() {
    local out=""
    local prev=""
    for arg in "$@"; do
      if [[ $prev == "-o" ]]; then
        out="$arg"
      fi
      prev="$arg"
    done
    echo "$(($(cat "$BUILD_COUNT_FILE") + 1))" >"$BUILD_COUNT_FILE"
    touch "$out"
    chmod +x "$out"
  }

  # shellcheck disable=SC2317 # Why: called indirectly as a stub.
  devbase_version_from_git() { return 1; }

  # shellcheck disable=SC2317 # Why: called indirectly as a stub.
  git() {
    case "$3" in
    rev-parse) echo "$REV" ;;
    status) [[ -n $DIRTY_DIFF ]] && echo " M some-file" ;; # empty -> a clean working tree.
    diff) echo "$DIRTY_DIFF" ;;
    ls-files) : ;; # no untracked files in these stubs.
    esac
  }

  BUILD_COUNT_FILE="$build_count_file"
  REV="$rev"
  DIRTY_DIFF="$dirty_diff"
  export BUILD_COUNT_FILE REV DIRTY_DIFF
}

@test "devbase_cached_source_binary builds once and reuses the cached binary on a second call" {
  # shellcheck disable=SC2329 # Why: called indirectly as a stub.
  devbase_tree_root() { echo "/fake/tree"; }
  local build_count_file="$DEVBASE_CACHED_BINARY_STORAGE_PATH/count"
  stub_go_git "$build_count_file"

  first="$(devbase_cached_source_binary)"
  second="$(devbase_cached_source_binary)"

  assert_equal "$first" "$second"
  assert_equal "$(cat "$build_count_file")" "1"
}

@test "devbase_cached_source_binary rebuilds when the source tree's commit changes" {
  local build_count_file="$DEVBASE_CACHED_BINARY_STORAGE_PATH/count"
  stub_go_git "$build_count_file"

  # shellcheck disable=SC2329 # Why: called indirectly as a stub.
  devbase_tree_root() { echo "/fake/tree/one"; }
  first="$(devbase_cached_source_binary)"

  # shellcheck disable=SC2329 # Why: called indirectly as a stub.
  devbase_tree_root() { echo "/fake/tree/two"; }
  stub_go_git "$build_count_file" "differentcommit"
  second="$(devbase_cached_source_binary)"

  assert_not_equal "$first" "$second"
  assert_equal "$(cat "$build_count_file")" "2"
}

# Regression test: two different uncommitted edits at the same commit
# used to share one cache key, so the second edit reused the first
# edit's stale binary.
@test "devbase_cached_source_binary rebuilds when uncommitted changes differ at the same commit" {
  local build_count_file="$DEVBASE_CACHED_BINARY_STORAGE_PATH/count"
  # shellcheck disable=SC2329 # Why: called indirectly as a stub.
  devbase_tree_root() { echo "/fake/tree"; }

  stub_go_git "$build_count_file" "deadbeefcafe" "diff content A"
  first="$(devbase_cached_source_binary)"

  stub_go_git "$build_count_file" "deadbeefcafe" "diff content B"
  second="$(devbase_cached_source_binary)"

  assert_not_equal "$first" "$second"
  assert_equal "$(cat "$build_count_file")" "2"
}

@test "devbase_cached_source_binary reuses the cached binary for identical uncommitted changes" {
  local build_count_file="$DEVBASE_CACHED_BINARY_STORAGE_PATH/count"
  # shellcheck disable=SC2329 # Why: called indirectly as a stub.
  devbase_tree_root() { echo "/fake/tree"; }

  stub_go_git "$build_count_file" "deadbeefcafe" "same diff content"
  first="$(devbase_cached_source_binary)"
  second="$(devbase_cached_source_binary)"

  assert_equal "$first" "$second"
  assert_equal "$(cat "$build_count_file")" "1"
}

@test "devbase_cli builds once and reuses the cached binary across calls" {
  # shellcheck disable=SC2329 # Why: called indirectly as a stub.
  devbase_cli_version() { echo "local"; }
  # shellcheck disable=SC2329 # Why: called indirectly as a stub.
  devbase_tree_root() { echo "/fake/tree"; }
  local build_count_file="$DEVBASE_CACHED_BINARY_STORAGE_PATH/count"
  stub_go_git "$build_count_file"

  run devbase_cli yq
  assert_success
  run devbase_cli yq
  assert_success

  assert_equal "$(cat "$build_count_file")" "1"
}

@test "devbase_cached_source_binary builds into a temporary file and leaves only the cached binary" {
  # shellcheck disable=SC2329 # Why: called indirectly as a stub.
  devbase_tree_root() { echo "/fake/tree"; }
  stub_go_git "$DEVBASE_CACHED_BINARY_STORAGE_PATH/count"
  # shellcheck disable=SC2329 # Why: called indirectly as a stub.
  devbase_build_from_source() {
    echo "$1" >"$BATS_TEST_TMPDIR/build_target"
    touch "$1"
  }

  cached="$(devbase_cached_source_binary)"

  assert_not_equal "$(cat "$BATS_TEST_TMPDIR/build_target")" "$cached"
  assert [ -x "$cached" ]
  assert_equal "$(ls "$(dirname "$cached")")" "$(basename "$cached")"
}

@test "devbase_cached_source_binary rebuilds on every call when go.mod has a local replace" {
  tree="$(mktempdir devbase-external-XXXXXX)"
  printf 'module x\n\nreplace github.com/getoutreach/gobox => ../gobox\n' >"$tree/go.mod"
  # shellcheck disable=SC2329 # Why: called indirectly as a stub.
  devbase_tree_root() { echo "$tree"; }
  local build_count_file="$DEVBASE_CACHED_BINARY_STORAGE_PATH/count"
  stub_go_git "$build_count_file"

  first="$(devbase_cached_source_binary)"
  second="$(devbase_cached_source_binary)"

  assert_equal "$first" "$second"
  assert_equal "$(cat "$build_count_file")" "2"
  rm -rf "$tree"
}

@test "devbase_cached_source_binary uses separate binaries for different trees with external inputs" {
  tree_a="$(mktempdir devbase-external-XXXXXX)"
  tree_b="$(mktempdir devbase-external-XXXXXX)"
  for t in "$tree_a" "$tree_b"; do
    printf 'module x\n\nreplace github.com/getoutreach/gobox => ../gobox\n' >"$t/go.mod"
  done
  stub_go_git "$DEVBASE_CACHED_BINARY_STORAGE_PATH/count"

  # shellcheck disable=SC2329 # Why: called indirectly as a stub.
  devbase_tree_root() { echo "$tree_a"; }
  first="$(devbase_cached_source_binary)"
  # shellcheck disable=SC2329 # Why: called indirectly as a stub.
  devbase_tree_root() { echo "$tree_b"; }
  second="$(devbase_cached_source_binary)"

  assert_not_equal "$first" "$second"
  rm -rf "$tree_a" "$tree_b"
}

@test "devbase_tree_has_external_build_inputs detects a go.work in a parent directory" {
  tree="$(mktempdir devbase-external-XXXXXX)"
  mkdir -p "$tree/devbase"
  echo "module x" >"$tree/devbase/go.mod"

  run devbase_tree_has_external_build_inputs "$tree/devbase"
  assert_failure

  touch "$tree/go.work"
  run devbase_tree_has_external_build_inputs "$tree/devbase"
  assert_success

  GOWORK=off run devbase_tree_has_external_build_inputs "$tree/devbase"
  assert_failure
  rm -rf "$tree"
}

@test "devbase_version_from_git ignores a deleted service.yaml" {
  make_git_repo
  echo "name: devbase" >"$repo/service.yaml"
  git -C "$repo" add service.yaml
  git -C "$repo" commit -q -m two
  git -C "$repo" tag v1.2.3
  rm "$repo/service.yaml"

  run devbase_version_from_git "$repo"
  assert_success
  assert_output "v1.2.3"

  echo x >"$repo/new"
  run devbase_version_from_git "$repo"
  assert_output "v1.2.3+dirty"
}

@test "devbase_prune_source_binaries removes only entries older than the limit" {
  base="$DEVBASE_CACHED_BINARY_STORAGE_PATH/devbase-src"
  mkdir -p "$base/old" "$base/new"
  touch "$base/old/devbase-src" "$base/new/devbase-src"
  touch -d '30 days ago' "$base/old" 2>/dev/null || touch -t 202001010000 "$base/old"

  run devbase_prune_source_binaries
  assert_success
  assert [ ! -e "$base/old" ]
  assert [ -e "$base/new/devbase-src" ]
}
