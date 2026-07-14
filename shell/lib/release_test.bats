#!/usr/bin/env bats
# shellcheck disable=SC2155

load release.sh
load bootstrap.sh
load version.sh

bats_load_library "bats-support/load.bash"
bats_load_library "bats-assert/load.bash"

# Note: base resolution is unit-tested here. The "No changes to release" no-op
# guard for an ancestor-of-base branch (the ancestor-of-base no-op guard) lives
# in dryrun.sh, an orchestration script exercised by the CI/manual dry-run path
# rather than bats.

setup() {
  REPO="$(mktemp -d)"
  git -C "$REPO" init -q
  git -C "$REPO" config user.email t@t.io
  git -C "$REPO" config user.name t
  git -C "$REPO" config tag.gpgsign false
  git -C "$REPO" commit -q --allow-empty -m "root"
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

@test "feature branch ahead of main resolves to default branch" {
  prereleases_on
  git -C "$REPO" checkout -q -b feature
  git -C "$REPO" commit -q --allow-empty -m "work"
  cd "$REPO"
  run resolve_release_base_branch "$REPO" feature main
  assert_output "main"
}

@test "ancestor branch with prereleases enabled resolves to release" {
  prereleases_on
  git -C "$REPO" tag rc
  git -C "$REPO" checkout -q main
  git -C "$REPO" commit -q --allow-empty -m "newer"
  git -C "$REPO" checkout -q -b tmp rc
  cd "$REPO"
  run resolve_release_base_branch "$REPO" tmp main
  assert_output "release"
}

@test "ancestor branch with prereleases disabled resolves to default branch" {
  prereleases_off
  git -C "$REPO" tag rc
  git -C "$REPO" checkout -q main
  git -C "$REPO" commit -q --allow-empty -m "newer"
  git -C "$REPO" checkout -q -b tmp rc
  cd "$REPO"
  run resolve_release_base_branch "$REPO" tmp main
  assert_output "main"
}

@test "RELEASE_BASE_BRANCH override wins" {
  prereleases_on
  git -C "$REPO" checkout -q -b feature
  cd "$REPO"
  RELEASE_BASE_BRANCH=custom run resolve_release_base_branch "$REPO" feature main
  assert_output "custom"
}

@test "rc-style branch one commit ahead of main resolves to default branch" {
  prereleases_on
  git -C "$REPO" checkout -q -b tmp-rc
  git -C "$REPO" commit -q --allow-empty -m "chore: Release RC"
  cd "$REPO"
  run resolve_release_base_branch "$REPO" tmp-rc main
  assert_output "main"
}

@test "release_commit_message concatenates messages in order" {
  git -C "$REPO" checkout -q -b feature
  git -C "$REPO" commit -q --allow-empty -m "first"
  git -C "$REPO" commit -q --allow-empty -m "second"
  run release_commit_message "$REPO" main feature
  assert_line --index 0 "first"
  assert_line --index 1 "second"
}

@test "release_commit_message is empty when head is an ancestor of base" {
  git -C "$REPO" tag rc
  git -C "$REPO" commit -q --allow-empty -m "newer on main"
  git -C "$REPO" checkout -q -b tmp rc
  run release_commit_message "$REPO" main tmp
  assert_output ""
}

@test "release_has_changes returns 0 when head has a real delta over base" {
  git -C "$REPO" checkout -q -b feature
  echo "feature work" >"$REPO/work.txt"
  git -C "$REPO" add work.txt
  git -C "$REPO" commit -q -m "feature work"
  run release_has_changes "$REPO" main feature
  [ "$status" -eq 0 ]
}

@test "release_has_changes returns 1 when head is an ancestor of base" {
  git -C "$REPO" tag rc
  git -C "$REPO" commit -q --allow-empty -m "newer on main"
  git -C "$REPO" checkout -q -b tmp rc
  run release_has_changes "$REPO" main tmp
  [ "$status" -eq 1 ]
}

@test "release_has_changes returns 1 when head change is already in base (empty-net)" {
  echo "shared" >"$REPO/shared.txt"
  git -C "$REPO" checkout -q -b feature
  git -C "$REPO" add shared.txt
  git -C "$REPO" commit -q -m "add shared on feature"
  git -C "$REPO" checkout -q main
  echo "shared" >"$REPO/shared.txt"
  git -C "$REPO" add shared.txt
  git -C "$REPO" commit -q -m "add identical shared on main"
  run release_has_changes "$REPO" main feature
  [ "$status" -eq 1 ]
}

@test "release_has_changes returns 2 and reports on a divergent conflict" {
  printf 'line1\n' >"$REPO/f.txt"
  git -C "$REPO" add f.txt
  git -C "$REPO" commit -q -m "base line"
  git -C "$REPO" checkout -q -b feature
  printf 'feature-version\n' >"$REPO/f.txt"
  git -C "$REPO" commit -q -am "feature change"
  git -C "$REPO" checkout -q main
  printf 'main-version\n' >"$REPO/f.txt"
  git -C "$REPO" commit -q -am "main change"
  run release_has_changes "$REPO" main feature
  [ "$status" -eq 2 ]
  # required report fields
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

@test "squash_branch commits the delta onto base with the given message" {
  git -C "$REPO" checkout -q -b feature
  echo "feature work" >"$REPO/work.txt"
  git -C "$REPO" add work.txt
  git -C "$REPO" commit -q -m "feature work"

  run release_has_changes "$REPO" main feature
  [ "$status" -eq 0 ]

  squash_branch "$REPO" main feature "squashed message"

  # base (main) now has a new commit with the message and the merged file.
  run git -C "$REPO" log -1 --format=%B main
  assert_output --partial "squashed message"
  run git -C "$REPO" show "main:work.txt"
  assert_output "feature work"
}

# The following integration tests exercise the git-level guards that dryrun.sh
# relies on (base-ref existence and merge-base availability) against a locally
# created clone, replacing the deferred manual-CI note.

@test "base-ref guard: origin/release resolves when the release branch is present" {
  git -C "$REPO" branch release
  CLONE="$(mktemp -d)"
  git clone -q "$REPO" "$CLONE"
  run git -C "$CLONE" rev-parse --verify "origin/release"
  [ "$status" -eq 0 ]
  rm -rf "$CLONE"
}

@test "base-ref guard: origin/release fails to resolve when the release branch is missing" {
  CLONE="$(mktemp -d)"
  git clone -q "$REPO" "$CLONE"
  run git -C "$CLONE" rev-parse --verify "origin/release"
  [ "$status" -ne 0 ]
  rm -rf "$CLONE"
}

@test "merge-base guard: a shallow clone with no common history has no merge-base" {
  # Build a second root so the two branches share no common ancestor.
  UNRELATED="$(mktemp -d)"
  git -C "$UNRELATED" init -q
  git -C "$UNRELATED" config user.email t@t.io
  git -C "$UNRELATED" config user.name t
  git -C "$UNRELATED" commit -q --allow-empty -m "unrelated root"
  git -C "$UNRELATED" branch -M other

  # Import the unrelated branch into $REPO without a shared ancestor.
  git -C "$REPO" fetch -q "$UNRELATED" other:other
  run git -C "$REPO" merge-base main other
  [ "$status" -ne 0 ]
  rm -rf "$UNRELATED"
}
