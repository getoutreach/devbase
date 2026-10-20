#!/usr/bin/env bats
# shellcheck disable=SC2155

load release.sh
load bootstrap.sh
load version.sh
load test_helper.sh

bats_load_library "bats-support/load.bash"
bats_load_library "bats-assert/load.bash"

# Required for `run --separate-stderr`.
bats_require_minimum_version 1.5.0

# Note: base resolution and the preview decision (release_preview_action) are
# unit-tested here. dryrun.sh only maps that decision to commands and is
# exercised by the CI/manual dry-run path rather than bats.

setup() {
  REPO="$(mktempdir devbase-release-XXXXXX)"
  git -C "$REPO" init --quiet
  git -C "$REPO" config user.email t@t.io
  git -C "$REPO" config user.name t
  git -C "$REPO" config tag.gpgsign false
  git -C "$REPO" commit --quiet --allow-empty --message "root"
  git -C "$REPO" branch -M main
}

teardown() {
  rm -rf "$REPO"
}

prereleases_on() {
  printf 'arguments:\n  releaseOptions:\n    enablePrereleases: true\n    prereleasesBranch: main\n' >"$REPO/service.yaml"
}

prereleases_off() {
  printf 'arguments:\n  releaseOptions:\n    enablePrereleases: false\n' >"$REPO/service.yaml"
}

# feature_adds_work leaves the repo on a new "feature" branch with one commit
# that adds work.txt, using $1 as the commit message.
feature_adds_work() {
  git -C "$REPO" checkout --quiet -b feature
  echo "feature work" >"$REPO/work.txt"
  git -C "$REPO" add work.txt
  git -C "$REPO" commit --quiet --message "$1"
}

# squash_merge_feature lands the "feature" branch's work.txt on main as a new
# commit, as a squash merge would.
squash_merge_feature() {
  git -C "$REPO" checkout --quiet main
  echo "feature work" >"$REPO/work.txt"
  git -C "$REPO" add work.txt
  git -C "$REPO" commit --quiet --message "feat!: feature work (#1)"
}

# ancestor_head leaves the repo on a "tmp" branch that is an ancestor of main.
ancestor_head() {
  git -C "$REPO" tag rc
  git -C "$REPO" commit --quiet --allow-empty --message "newer on main"
  git -C "$REPO" checkout --quiet -b tmp rc
}

# diverging_conflict leaves "main" and "feature" with conflicting edits to f.txt.
diverging_conflict() {
  printf 'line1\n' >"$REPO/f.txt"
  git -C "$REPO" add f.txt
  git -C "$REPO" commit --quiet --message "base line"
  git -C "$REPO" checkout --quiet -b feature
  printf 'feature-version\n' >"$REPO/f.txt"
  git -C "$REPO" commit --quiet --all --message "feature change"
  git -C "$REPO" checkout --quiet main
  printf 'main-version\n' >"$REPO/f.txt"
  git -C "$REPO" commit --quiet --all --message "main change"
}

@test "feature branch ahead of main resolves to default branch" {
  prereleases_on
  git -C "$REPO" checkout --quiet -b feature
  git -C "$REPO" commit --quiet --allow-empty --message "work"
  cd "$REPO"
  run resolve_release_base_branch "$REPO" feature main
  assert_output "main"
}

@test "ancestor branch with prereleases enabled resolves to release" {
  prereleases_on
  git -C "$REPO" tag rc
  git -C "$REPO" checkout --quiet main
  git -C "$REPO" commit --quiet --allow-empty --message "newer"
  git -C "$REPO" checkout --quiet -b tmp rc
  cd "$REPO"
  run resolve_release_base_branch "$REPO" tmp main
  assert_output "release"
}

@test "ancestor branch with prereleases disabled resolves to default branch" {
  prereleases_off
  git -C "$REPO" tag rc
  git -C "$REPO" checkout --quiet main
  git -C "$REPO" commit --quiet --allow-empty --message "newer"
  git -C "$REPO" checkout --quiet -b tmp rc
  cd "$REPO"
  run resolve_release_base_branch "$REPO" tmp main
  assert_output "main"
}

@test "RELEASE_BASE_BRANCH override wins" {
  prereleases_on
  git -C "$REPO" checkout --quiet -b feature
  cd "$REPO"
  RELEASE_BASE_BRANCH=custom run resolve_release_base_branch "$REPO" feature main
  assert_output "custom"
}

@test "rc-style branch one commit ahead of main resolves to default branch" {
  prereleases_on
  git -C "$REPO" checkout --quiet -b tmp-rc
  git -C "$REPO" commit --quiet --allow-empty --message "chore: Release RC"
  cd "$REPO"
  run resolve_release_base_branch "$REPO" tmp-rc main
  assert_output "main"
}

@test "release_commit_message concatenates messages in order" {
  git -C "$REPO" checkout --quiet -b feature
  git -C "$REPO" commit --quiet --allow-empty --message "first"
  git -C "$REPO" commit --quiet --allow-empty --message "second"
  run release_commit_message "$REPO" main feature
  assert_line --index 0 "first"
  assert_line --index 1 "second"
}

@test "release_commit_message is empty when head is an ancestor of base" {
  ancestor_head
  run release_commit_message "$REPO" main tmp
  assert_output ""
}

@test "release_has_changes returns 0 when head has a real delta over base" {
  feature_adds_work "feature work"
  run release_has_changes "$REPO" main feature
  [ "$status" -eq 0 ]
}

@test "release_has_changes returns 1 when head is an ancestor of base" {
  ancestor_head
  run release_has_changes "$REPO" main tmp
  [ "$status" -eq 1 ]
}

@test "release_has_changes returns 1 when head change is already in base (empty-net)" {
  echo "shared" >"$REPO/shared.txt"
  git -C "$REPO" checkout --quiet -b feature
  git -C "$REPO" add shared.txt
  git -C "$REPO" commit --quiet --message "add shared on feature"
  git -C "$REPO" checkout --quiet main
  echo "shared" >"$REPO/shared.txt"
  git -C "$REPO" add shared.txt
  git -C "$REPO" commit --quiet --message "add identical shared on main"
  run release_has_changes "$REPO" main feature
  [ "$status" -eq 1 ]
}

@test "release_head_has_net_changes returns 0 when head was squash-merged into base" {
  feature_adds_work "feature work"
  squash_merge_feature
  run release_has_changes "$REPO" main feature
  [ "$status" -eq 1 ]
  run release_head_has_net_changes "$REPO" main feature
  [ "$status" -eq 0 ]
}

@test "release_head_has_net_changes returns 0 when head was rebase-merged into base" {
  feature_adds_work "feat!: feature work"
  git -C "$REPO" checkout --quiet main
  git -C "$REPO" commit --quiet --allow-empty --message "newer on main"
  git -C "$REPO" cherry-pick feature >/dev/null
  run release_has_changes "$REPO" main feature
  [ "$status" -eq 1 ]
  run release_head_has_net_changes "$REPO" main feature
  [ "$status" -eq 0 ]
}

@test "release_head_has_net_changes returns 0 when git diff fails" {
  feature_adds_work "feature work"
  # Remove head's tree object: merge-base only reads commits, but diff needs the tree.
  local tree
  tree="$(git -C "$REPO" rev-parse "feature^{tree}")"
  rm -f "$REPO/.git/objects/${tree:0:2}/${tree:2}"
  run git -C "$REPO" diff --quiet main feature
  [ "$status" -gt 1 ]
  run release_head_has_net_changes "$REPO" main feature
  [ "$status" -eq 0 ]
}

@test "release_head_has_net_changes returns 0 when the merge-base cannot be determined" {
  run release_head_has_net_changes "$REPO" main does-not-exist
  [ "$status" -eq 0 ]
}

@test "release_head_has_net_changes returns 1 when head is an ancestor of base" {
  ancestor_head
  run release_head_has_net_changes "$REPO" main tmp
  [ "$status" -eq 1 ]
}

@test "release_head_has_net_changes returns 1 when head only adds empty commits" {
  git -C "$REPO" checkout --quiet -b feature
  git -C "$REPO" commit --quiet --allow-empty --message "chore: nothing"
  git -C "$REPO" checkout --quiet main
  git -C "$REPO" commit --quiet --allow-empty --message "newer on main"
  run release_head_has_net_changes "$REPO" main feature
  [ "$status" -eq 1 ]
}

@test "release_has_changes returns 2 and reports on a divergent conflict" {
  diverging_conflict
  run release_has_changes "$REPO" main feature
  [ "$status" -eq 2 ]
  assert_output --partial "main"           # base ref named in header
  assert_output --partial "feature"        # head ref named in header
  assert_output --partial "f.txt"          # conflicted path
  assert_output --partial "cannot preview" # conflict report header
  assert_output --partial "merge-base:"    # merge-base field
}

@test "release_has_changes returns 2 (operational error) for a nonexistent ref" {
  run release_has_changes "$REPO" main does-not-exist
  [ "$status" -eq 2 ]
  assert_output --partial "operational error"
  assert_output --partial "does-not-exist"
}

@test "release_preview_action returns squash when head has a real delta over base" {
  feature_adds_work "feature work"
  run release_preview_action "$REPO" main feature
  [ "$status" -eq 0 ]
  assert_output "squash"
}

@test "release_preview_action returns merged when head was squash-merged into base" {
  feature_adds_work "feature work"
  squash_merge_feature
  run release_preview_action "$REPO" main feature
  [ "$status" -eq 0 ]
  assert_output "merged"
}

@test "release_preview_action returns none when head is an ancestor of base" {
  ancestor_head
  run release_preview_action "$REPO" main tmp
  [ "$status" -eq 0 ]
  assert_output "none"
}

@test "release_preview_action returns error on a merge conflict" {
  diverging_conflict
  run --separate-stderr release_preview_action "$REPO" main feature
  [ "$status" -eq 0 ]
  assert_output "error"
  [[ $stderr == *"cannot preview"* ]]
}

@test "release_preview_action returns none when head only adds empty commits" {
  git -C "$REPO" checkout --quiet -b feature
  git -C "$REPO" commit --quiet --allow-empty --message "chore: nothing"
  git -C "$REPO" checkout --quiet main
  git -C "$REPO" commit --quiet --allow-empty --message "newer on main"
  run release_preview_action "$REPO" main feature
  [ "$status" -eq 0 ]
  assert_output "none"
}

@test "release_preview_action returns none when head was merge-committed into base (known limit)" {
  feature_adds_work "feature work"
  git -C "$REPO" checkout --quiet main
  git -C "$REPO" merge --quiet --no-ff feature --message "Merge feature"
  run release_preview_action "$REPO" main feature
  [ "$status" -eq 0 ]
  assert_output "none"
}

@test "release_preview_action returns error for a nonexistent ref" {
  run --separate-stderr release_preview_action "$REPO" main does-not-exist
  [ "$status" -eq 0 ]
  assert_output "error"
  [[ $stderr == *"operational error"* ]]
}

@test "squash_branch commits the delta onto base with the given message" {
  feature_adds_work "feature work"

  run release_has_changes "$REPO" main feature
  [ "$status" -eq 0 ]

  squash_branch "$REPO" main feature "squashed message"

  run git -C "$REPO" log -1 --format=%B main
  assert_output --partial "squashed message"
  run git -C "$REPO" show "main:work.txt"
  assert_output "feature work"
}

# Integration tests for the git-level guards dryrun.sh relies on: base-ref
# existence and merge-base availability.

@test "base-ref guard: origin/release resolves when the release branch is present" {
  git -C "$REPO" branch release
  CLONE="$(mktempdir devbase-release-clone-XXXXXX)"
  git clone --quiet "$REPO" "$CLONE"
  run git -C "$CLONE" rev-parse --verify "origin/release"
  [ "$status" -eq 0 ]
  rm -rf "$CLONE"
}

@test "base-ref guard: origin/release fails to resolve when the release branch is missing" {
  CLONE="$(mktempdir devbase-release-clone-XXXXXX)"
  git clone --quiet "$REPO" "$CLONE"
  run git -C "$CLONE" rev-parse --verify "origin/release"
  [ "$status" -ne 0 ]
  rm -rf "$CLONE"
}

@test "merge-base guard: a shallow clone with no common history has no merge-base" {
  # Build a second root so the two branches share no common ancestor.
  UNRELATED="$(mktempdir devbase-release-unrelated-XXXXXX)"
  git -C "$UNRELATED" init --quiet
  git -C "$UNRELATED" config user.email t@t.io
  git -C "$UNRELATED" config user.name t
  git -C "$UNRELATED" commit --quiet --allow-empty --message "unrelated root"
  git -C "$UNRELATED" branch -M other

  # Import the unrelated branch into $REPO without a shared ancestor.
  git -C "$REPO" fetch --quiet "$UNRELATED" other:other
  run git -C "$REPO" merge-base main other
  [ "$status" -ne 0 ]
  rm -rf "$UNRELATED"
}
