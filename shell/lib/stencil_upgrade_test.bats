#!/usr/bin/env bats

bats_require_minimum_version 1.5.0

load stencil_upgrade.sh
load test_helper.sh

bats_load_library "bats-support/load.bash"
bats_load_library "bats-assert/load.bash"

setup() {
  TEST_DIR="$(mktempdir stencil-upgrade-XXXXXX)"
  setup_command_stubs
}

teardown() {
  # Some tests cd into TEST_DIR, which the next line removes.
  cd "$BATS_TEST_DIRNAME"
  teardown_command_stubs
  rm -rf "$TEST_DIR"
}

# commit_as NAME EMAIL SUBJECT makes an empty commit in the current repo.
commit_as() {
  git -c user.name="$1" -c user.email="$2" commit --allow-empty -q -m "$3"
}

# write_lock VERSION... writes a stencil.lock with one module per version.
write_lock() {
  local i=0
  {
    echo "modules:"
    for version in "$@"; do
      echo "  - name: example.com/module$((i++))"
      echo "    version: $version"
    done
  } >"$TEST_DIR/stencil.lock"
}

@test "stencil_upgrade_branch_name should use the UTC date" {
  stub_command date "2026-11-21"

  run stencil_upgrade_branch_name
  assert_success
  assert_output "stencil-upgrade/2026-11-21"
  assert_stub_called date -u +%Y-%m-%d
}

@test "stencil_upgrade_open_prs should keep only exact prefix matches" {
  stub_command gh "$(printf '%s\n' \
    stencil-upgrade/2026-11-20 stencil-upgrade-other/x stencilbot/2026-11-19)"

  run stencil_upgrade_open_prs "stencil-upgrade/"
  assert_success
  assert_output "stencil-upgrade/2026-11-20"
  assert_stub_called gh pr list --state open --limit 200 \
    --search head:stencil-upgrade --json headRefName --jq .[].headRefName
}

@test "stencil_upgrade_open_prs should fail when gh fails" {
  stub_command gh "boom" 1

  run stencil_upgrade_open_prs "stencil-upgrade/"
  assert_failure
}

@test "stencil_upgrade_only_bot_commits should pass for bot commits only" {
  cd "$TEST_DIR"
  git init -q -b main
  commit_as human human@example.com base
  commit_as bot bot@example.com one
  commit_as bot bot@example.com two

  run stencil_upgrade_only_bot_commits main~2 main bot bot@example.com
  assert_success
  assert_output ""
}

@test "stencil_upgrade_only_bot_commits should fail and name a human commit" {
  cd "$TEST_DIR"
  git init -q -b main
  commit_as human human@example.com base
  commit_as bot bot@example.com one
  commit_as human human@example.com "human fix"

  run stencil_upgrade_only_bot_commits main~2 main bot bot@example.com
  assert_failure
  assert_output --partial "human fix"
  assert_output --partial "human <human@example.com>"
}

@test "stencil_upgrade_only_bot_commits should pass for an empty range" {
  cd "$TEST_DIR"
  git init -q -b main
  commit_as human human@example.com base

  run stencil_upgrade_only_bot_commits main main bot bot@example.com
  assert_success
  assert_output ""
}

@test "stencil_upgrade_only_bot_commits should fail for a bad ref" {
  cd "$TEST_DIR"
  git init -q -b main
  commit_as bot bot@example.com one

  run stencil_upgrade_only_bot_commits missing main bot bot@example.com
  assert_failure
}

@test "stencil_upgrade_force_pr should be true when the flag is true" {
  write_lock v1.2.0

  run stencil_upgrade_force_pr true "$TEST_DIR/stencil.lock"
  assert_success
  assert_output "true"
}

@test "stencil_upgrade_force_pr should be true when a module is unstable" {
  write_lock v1.2.0 unstable

  run stencil_upgrade_force_pr false "$TEST_DIR/stencil.lock"
  assert_success
  assert_output "true"
}

@test "stencil_upgrade_force_pr should be false otherwise" {
  write_lock v1.2.0 rc local

  run stencil_upgrade_force_pr false "$TEST_DIR/stencil.lock"
  assert_success
  assert_output "false"
}

@test "stencil_upgrade_version_change_count should ignore unchanged unstable entries" {
  cat >"$TEST_DIR/changes.json" <<'JSON'
[
  {"module": "example.com/a", "old": "v1.0.0", "new": "v1.1.0"},
  {"module": "example.com/b", "old": "unstable", "new": "unstable", "unstable": true},
  {"module": "example.com/c", "old": "v2.0.0", "new": "unstable", "unstable": true}
]
JSON

  run stencil_upgrade_version_change_count "$TEST_DIR/changes.json"
  assert_success
  assert_output "2"
}

@test "stencil_upgrade_version_change_count should be 0 for no changes" {
  echo "[]" >"$TEST_DIR/changes.json"

  run stencil_upgrade_version_change_count "$TEST_DIR/changes.json"
  assert_success
  assert_output "0"
}

@test "stencil_upgrade_decide should follow the decision table" {
  local count force dirty want
  # count force_pr tree_dirty -> decision
  while read -r count force dirty want; do
    run stencil_upgrade_decide "$count" "$force" "$dirty"
    assert_success
    assert_output "$want"
  done <<'TABLE'
2 false false open
2 true false open
0 false false stop-no-changes
0 false true stop-no-changes
0 true true open
0 true false stop-empty
TABLE
}
