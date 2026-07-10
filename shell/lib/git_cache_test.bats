#!/usr/bin/env bash

# Required for `run --separate-stderr` (test 1), which isolates stdout.
bats_require_minimum_version 1.5.0

bats_load_library "bats-support/load.bash"
bats_load_library "bats-assert/load.bash"

load git_cache.sh
load logging.sh
load test_helper.sh

setup() {
  TESTROOT="$(mktempdir devbase-git-cache-XXXXXX)"

  # Build a local "origin" repo that mimics a schema repo layout, so tests
  # never touch the network. cache_git_repo clones it via a file:// URL.
  ORIGIN="$TESTROOT/origin"
  git init -q --initial-branch=main "$ORIGIN"
  git -C "$ORIGIN" config user.email "git-cache@devbase.test"
  git -C "$ORIGIN" config user.name "Test User"
  mkdir -p "$ORIGIN/v1.25.16-standalone-strict" \
    "$ORIGIN/v1.26.0-standalone-strict" "$ORIGIN/unwanted"
  echo '{"type":"object"}' >"$ORIGIN/v1.25.16-standalone-strict/deployment-apps-v1.json"
  echo '{"type":"object"}' >"$ORIGIN/v1.26.0-standalone-strict/deployment-apps-v1.json"
  echo 'junk' >"$ORIGIN/unwanted/big.txt"
  git -C "$ORIGIN" add -A
  git -C "$ORIGIN" commit -qm "initial schemas"

  # Redirect the cache root at the temp dir instead of $HOME/.outreach/.cache.
  DEVBASE_CACHE_DIR="$TESTROOT/cache"
}

teardown() {
  rm -rf "$TESTROOT"
}

@test "cache_git_repo without sparse paths clones the full working tree" {
  # Use --separate-stderr so $output is only the cache dir path; the function
  # and git clone log to stderr.
  run --separate-stderr cache_git_repo "file://$ORIGIN" kubeconform
  assert_success
  local cacheDir="$DEVBASE_CACHE_DIR/kubeconform/origin"
  assert_output "$cacheDir"
  assert [ -f "$cacheDir/v1.25.16-standalone-strict/deployment-apps-v1.json" ]
  assert [ -f "$cacheDir/v1.26.0-standalone-strict/deployment-apps-v1.json" ]
  assert [ -f "$cacheDir/unwanted/big.txt" ]
}

@test "cache_git_repo with sparse paths materializes only those directories" {
  run cache_git_repo "file://$ORIGIN" kubeconform v1.25.16-standalone-strict
  assert_success
  local cacheDir="$DEVBASE_CACHE_DIR/kubeconform/origin"
  assert [ -f "$cacheDir/v1.25.16-standalone-strict/deployment-apps-v1.json" ]
  assert [ ! -d "$cacheDir/v1.26.0-standalone-strict" ]
  assert [ ! -d "$cacheDir/unwanted" ]
}

@test "cache_git_repo accepts multiple sparse paths" {
  run cache_git_repo "file://$ORIGIN" kubeconform \
    v1.25.16-standalone-strict v1.26.0-standalone-strict
  assert_success
  local cacheDir="$DEVBASE_CACHE_DIR/kubeconform/origin"
  assert [ -d "$cacheDir/v1.25.16-standalone-strict" ]
  assert [ -d "$cacheDir/v1.26.0-standalone-strict" ]
  assert [ ! -d "$cacheDir/unwanted" ]
}

@test "cache_git_repo updates an existing sparse cache and preserves sparsity" {
  cache_git_repo "file://$ORIGIN" kubeconform v1.25.16-standalone-strict
  # Second call hits the update path (cache dir already exists).
  run cache_git_repo "file://$ORIGIN" kubeconform v1.25.16-standalone-strict
  assert_success
  local cacheDir="$DEVBASE_CACHE_DIR/kubeconform/origin"
  assert [ -f "$cacheDir/v1.25.16-standalone-strict/deployment-apps-v1.json" ]
  assert [ ! -d "$cacheDir/unwanted" ]
}

@test "cache_git_repo sparse update prints only the cache dir path on stdout" {
  # Warm the cache, then update. The update path must not leak git reset
  # output (e.g. "HEAD is now at ...") onto stdout; callers capture stdout as
  # the cache dir path, so it must be EXACTLY the path.
  cache_git_repo "file://$ORIGIN" kubeconform v1.25.16-standalone-strict
  run --separate-stderr cache_git_repo "file://$ORIGIN" kubeconform v1.25.16-standalone-strict
  assert_success
  local cacheDir="$DEVBASE_CACHE_DIR/kubeconform/origin"
  assert_output "$cacheDir"
}

@test "cache_git_repo non-sparse update prints only the cache dir path on stdout" {
  cache_git_repo "file://$ORIGIN" kubeconform
  run --separate-stderr cache_git_repo "file://$ORIGIN" kubeconform
  assert_success
  local cacheDir="$DEVBASE_CACHE_DIR/kubeconform/origin"
  assert_output "$cacheDir"
}

@test "cache_git_repo warns and reuses the checkout when a warm-cache fetch fails" {
  # Warm the cache, then make origin unreachable to simulate a transient
  # network failure on a later run. A usable local checkout already exists,
  # so the fetch failure must be tolerated: the function warns (on stderr)
  # and returns the cache dir with the previously-materialized schemas intact.
  cache_git_repo "file://$ORIGIN" kubeconform v1.25.16-standalone-strict
  rm -rf "$ORIGIN"

  run --separate-stderr cache_git_repo "file://$ORIGIN" kubeconform v1.25.16-standalone-strict
  assert_success
  local cacheDir="$DEVBASE_CACHE_DIR/kubeconform/origin"
  assert_output "$cacheDir"
  assert [ -f "$cacheDir/v1.25.16-standalone-strict/deployment-apps-v1.json" ]
  # The failure is reported clearly rather than leaking a bare git "fatal:".
  assert [ -n "$stderr" ]
}

@test "cache_git_repo self-heals a corrupt (non-git) cache dir" {
  # Simulate an interrupted clone: the cache dir exists but is not a git
  # repo. The old code took the update path and `git fetch` errored; the
  # function must instead treat this as a cache miss and re-clone.
  local cacheDir="$DEVBASE_CACHE_DIR/kubeconform/origin"
  mkdir -p "$cacheDir"
  echo 'partial junk' >"$cacheDir/leftover.txt"

  run --separate-stderr cache_git_repo "file://$ORIGIN" kubeconform v1.25.16-standalone-strict
  assert_success
  assert_output "$cacheDir"
  # Proves it re-cloned rather than aborting on the corrupt dir.
  assert [ -f "$cacheDir/v1.25.16-standalone-strict/deployment-apps-v1.json" ]
}
