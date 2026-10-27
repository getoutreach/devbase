#!/usr/bin/env bats

bats_require_minimum_version 1.5.0

bats_load_library "bats-support/load.bash"
bats_load_library "bats-assert/load.bash"

load logging.sh
load yq.sh
load test_helper.sh

setup() {
  # Isolate the binary cache per test so a stubbed "build" here never
  # collides with a real cached devbase binary on disk.
  DEVBASE_CACHED_BINARY_STORAGE_PATH="$(mktempdir devbase-yq-cache-XXXXXX)"
  export DEVBASE_CACHED_BINARY_STORAGE_PATH
  DEVBASE_YQ_BIN=""
}

teardown() {
  rm -rf "$DEVBASE_CACHED_BINARY_STORAGE_PATH"
  unset DEVBASE_CACHED_BINARY_STORAGE_PATH
}

# stub_go_git stubs `go` and `git` for yq_build_from_source: `git`
# reports a clean, fixed commit ("deadbeefcafe", or $2 when given), and
# `go build -o <path> ...` just creates an empty executable at <path>
# and records a call count in $1, instead of doing a real (slow,
# network-independent-but-still slow) build.
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

@test "yq_build_from_source builds once and reuses the cached binary on a second call" {
  # shellcheck disable=SC2329 # Why: called indirectly as a stub.
  devbase_tree_root() { echo "/fake/tree"; }
  local build_count_file="$DEVBASE_CACHED_BINARY_STORAGE_PATH/count"
  stub_go_git "$build_count_file"

  first="$(yq_build_from_source)"
  second="$(yq_build_from_source)"

  assert_equal "$first" "$second"
  assert_equal "$(cat "$build_count_file")" "1"
}

@test "yq_build_from_source rebuilds when the source tree's commit changes" {
  local build_count_file="$DEVBASE_CACHED_BINARY_STORAGE_PATH/count"
  stub_go_git "$build_count_file"

  # shellcheck disable=SC2329 # Why: called indirectly as a stub.
  devbase_tree_root() { echo "/fake/tree/one"; }
  first="$(yq_build_from_source)"

  # shellcheck disable=SC2329 # Why: called indirectly as a stub.
  devbase_tree_root() { echo "/fake/tree/two"; }
  stub_go_git "$build_count_file" "differentcommit"
  second="$(yq_build_from_source)"

  assert_not_equal "$first" "$second"
  assert_equal "$(cat "$build_count_file")" "2"
}

# TestYqBuildFromSourceContentAwareCacheKey is a regression test for a
# real bug found during review: two different uncommitted edits at the
# same commit used to collide on a single flat "dirty-<rev>" cache key,
# so the second edit silently reused the first edit's stale binary.
@test "yq_build_from_source rebuilds when uncommitted changes differ at the same commit" {
  local build_count_file="$DEVBASE_CACHED_BINARY_STORAGE_PATH/count"
  # shellcheck disable=SC2329 # Why: called indirectly as a stub.
  devbase_tree_root() { echo "/fake/tree"; }

  stub_go_git "$build_count_file" "deadbeefcafe" "diff content A"
  first="$(yq_build_from_source)"

  stub_go_git "$build_count_file" "deadbeefcafe" "diff content B"
  second="$(yq_build_from_source)"

  assert_not_equal "$first" "$second"
  assert_equal "$(cat "$build_count_file")" "2"
}

@test "yq_build_from_source reuses the cached binary for identical uncommitted changes" {
  local build_count_file="$DEVBASE_CACHED_BINARY_STORAGE_PATH/count"
  # shellcheck disable=SC2329 # Why: called indirectly as a stub.
  devbase_tree_root() { echo "/fake/tree"; }

  stub_go_git "$build_count_file" "deadbeefcafe" "same diff content"
  first="$(yq_build_from_source)"
  second="$(yq_build_from_source)"

  assert_equal "$first" "$second"
  assert_equal "$(cat "$build_count_file")" "1"
}

@test "yq_resolve_bin uses yq_build_from_source for a local/dev version" {
  # shellcheck disable=SC2329 # Why: called indirectly as a stub.
  devbase_cli_version() { echo "local"; }
  # shellcheck disable=SC2329 # Why: called indirectly as a stub.
  find_tool() { echo "/fake/go"; }
  local build_count_file="$DEVBASE_CACHED_BINARY_STORAGE_PATH/count"
  stub_go_git "$build_count_file"
  # shellcheck disable=SC2329 # Why: called indirectly as a stub.
  devbase_tree_root() { echo "/fake/tree"; }

  yq_resolve_bin
  assert [ -n "$DEVBASE_YQ_BIN" ]
  assert [ -x "$DEVBASE_YQ_BIN" ]
}

@test "yq_resolve_bin uses devbase_cli_binary for a tagged version" {
  # shellcheck disable=SC2329 # Why: called indirectly as a stub.
  devbase_cli_version() { echo "v2.40.0"; }
  # shellcheck disable=SC2317,SC2329 # Why: called indirectly as a stub.
  devbase_cli_binary() { echo "/fake/devbase-v2.40.0"; }

  yq_resolve_bin
  assert_equal "$DEVBASE_YQ_BIN" "/fake/devbase-v2.40.0"
}

@test "yq_resolve_bin does not re-resolve once DEVBASE_YQ_BIN is set" {
  DEVBASE_YQ_BIN="/already/resolved"
  # shellcheck disable=SC2317,SC2329 # Why: called indirectly as a stub; should never run.
  devbase_cli_version() {
    echo "unexpectedly called devbase_cli_version" >&2
    return 1
  }

  run yq_resolve_bin
  assert_success
  assert_equal "$DEVBASE_YQ_BIN" "/already/resolved"
}

# TestYqSourcingDoesNotClobberCallerVariables is a regression test for
# a real bug found during review: shell/cli.sh (sourced transitively by
# yq.sh) used to declare its own path in a bare $DIR/$LIB_DIR variable,
# which silently overwrote any caller's own $DIR/$LIB_DIR of the same
# name -- breaking shell/lib/docker.sh, which uses $DIR for its own
# later source lines. cli.sh's internal variables are namespaced now;
# this confirms a caller's own $DIR survives sourcing yq.sh.
@test "sourcing yq.sh does not clobber a caller's own DIR variable" {
  DIR="/caller/owns/this"
  source "$BATS_TEST_DIRNAME/yq.sh"
  assert_equal "$DIR" "/caller/owns/this"
}

@test "real devbase yq.sh caller (docker.sh) sources cleanly with no stderr output" {
  run bash -c "source '$BATS_TEST_DIRNAME/docker.sh' 2>&1"
  assert_success
  assert_output ""
}

# stub_gojq_env stubs the version and tool lookups behind yq_use_gojq:
# $1 is the devbase version, $2 is the path find_tool reports for go
# (empty means not found), and $3 is the same for gojq.
stub_gojq_env() {
  # Not local: the stubs below read these after this function returns.
  STUB_VERSION="$1"
  STUB_GO_PATH="$2"
  STUB_GOJQ_PATH="${3:-}"
  # shellcheck disable=SC2317,SC2329 # Why: called indirectly as a stub.
  devbase_cli_version() { echo "$STUB_VERSION"; }
  # shellcheck disable=SC2317,SC2329 # Why: called indirectly as a stub.
  find_tool() {
    local path=""
    case "$1" in
    go) path="$STUB_GO_PATH" ;;
    gojq) path="$STUB_GOJQ_PATH" ;;
    esac
    [[ -n $path ]] || return 1
    echo "$path"
  }
}

@test "yq_use_gojq succeeds for an unstable version without go" {
  stub_gojq_env local ""
  run yq_use_gojq
  assert_success
}

@test "yq_use_gojq fails when go is installed" {
  stub_gojq_env local /usr/bin/go
  run yq_use_gojq
  assert_failure
}

@test "yq_use_gojq fails for a tagged version even without go" {
  stub_gojq_env v2.40.0 ""
  run yq_use_gojq
  assert_failure
}

@test "yq_run_gojq runs gojq with --yaml-input" {
  fake_bin_dir="$(mktempdir devbase-yq-gojq-XXXXXX)"
  printf '#!/usr/bin/env bash\necho "gojq $*"\n' >"$fake_bin_dir/gojq"
  chmod +x "$fake_bin_dir/gojq"
  stub_gojq_env local "" "$fake_bin_dir/gojq"

  run yq_run_gojq .a file.yaml
  assert_success
  assert_output "gojq --yaml-input .a file.yaml"

  rm -rf "$fake_bin_dir"
}

@test "yq_run_gojq fails with a clear error when gojq is missing" {
  stub_gojq_env local ""
  run yq_run_gojq .a
  assert_failure
  assert_output --partial "neither go nor gojq found"
}

@test "yq_wrapper uses gojq instead of building devbase when go is missing" {
  stub_gojq_env local ""
  yq_run_gojq() { echo "gojq fallback $*"; }
  yq_resolve_bin() {
    echo "unexpected build" >&2
    return 1
  }

  run yq_wrapper .a
  assert_success
  assert_output "gojq fallback .a"
}

@test "yq_resolve_bin does not build devbase when go is missing" {
  stub_gojq_env local ""
  yq_build_from_source() {
    echo "unexpected build" >&2
    return 1
  }

  yq_resolve_bin
  # shellcheck disable=SC2031 # Why: yq_resolve_bin runs in this shell, not a subshell.
  assert_equal "$DEVBASE_YQ_BIN" ""
}

@test "yq_resolve_bin still resolves a tagged version when go is missing" {
  stub_gojq_env v2.40.0 ""
  # shellcheck disable=SC2317,SC2329 # Why: called indirectly as a stub.
  devbase_cli_binary() { echo "/fake/devbase-v2.40.0"; }

  yq_resolve_bin
  # shellcheck disable=SC2031 # Why: yq_resolve_bin runs in this shell, not a subshell.
  assert_equal "$DEVBASE_YQ_BIN" "/fake/devbase-v2.40.0"
}

@test "tomlq_wrapper runs devbase tomlq with the given arguments" {
  fake_bin_dir="$(mktempdir devbase-yq-tomlq-XXXXXX)"
  printf '#!/usr/bin/env bash\necho "devbase $*"\n' >"$fake_bin_dir/devbase"
  chmod +x "$fake_bin_dir/devbase"
  stub_gojq_env local /usr/bin/go
  DEVBASE_YQ_BIN="$fake_bin_dir/devbase"

  run tomlq_wrapper -r .a file.toml
  assert_success
  assert_output "devbase tomlq -r .a file.toml"

  rm -rf "$fake_bin_dir"
}

@test "tomlq_wrapper fails with a clear error instead of falling back to gojq" {
  stub_gojq_env local ""

  run tomlq_wrapper .a
  assert_failure
  assert_output --partial "cannot run tomlq"
}
