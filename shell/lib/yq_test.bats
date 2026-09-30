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
    status) : ;; # no output -> a clean working tree.
    esac
  }

  BUILD_COUNT_FILE="$build_count_file"
  REV="$rev"
  export BUILD_COUNT_FILE REV
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

@test "yq_resolve_bin uses yq_build_from_source for a local/dev version" {
  # shellcheck disable=SC2329 # Why: called indirectly as a stub.
  devbase_cli_version() { echo "local"; }
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
