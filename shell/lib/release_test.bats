#!/usr/bin/env bats
# shellcheck disable=SC2155

load release.sh
load bootstrap.sh

bats_load_library "bats-support/load.bash"
bats_load_library "bats-assert/load.bash"

# Note: base resolution is unit-tested here. The "No changes to release" no-op
# guard for an ancestor-of-base branch (spec case 6) lives in dryrun.sh, an
# orchestration script exercised by the CI/manual dry-run path rather than bats.

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
