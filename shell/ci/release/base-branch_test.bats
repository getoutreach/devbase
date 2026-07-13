#!/usr/bin/env bats
# shellcheck disable=SC2155

load base-branch.sh

bats_load_library "bats-support/load.bash"
bats_load_library "bats-assert/load.bash"

setup() {
  REPO="$(mktemp -d)"
  YAML="$(mktemp)"
  git -C "$REPO" init -q
  git -C "$REPO" config user.email t@t.io
  git -C "$REPO" config user.name t
  git -C "$REPO" config tag.gpgsign false
  git -C "$REPO" commit -q --allow-empty -m "root"
  git -C "$REPO" branch -M main
}

teardown() {
  rm -rf "$REPO" "$YAML"
}

prereleases_on() {
  printf 'arguments:\n  releaseOptions:\n    enablePrereleases: true\n    prereleasesBranch: main\n' >"$YAML"
}

prereleases_off() {
  printf 'arguments:\n  releaseOptions:\n    enablePrereleases: false\n' >"$YAML"
}

@test "feature branch ahead of main resolves to default branch" {
  prereleases_on
  git -C "$REPO" checkout -q -b feature
  git -C "$REPO" commit -q --allow-empty -m "work"
  run resolve_release_base_branch "$REPO" feature main "$YAML"
  assert_output "main"
}

@test "ancestor branch with prereleases enabled resolves to release" {
  prereleases_on
  git -C "$REPO" tag rc
  git -C "$REPO" checkout -q main
  git -C "$REPO" commit -q --allow-empty -m "newer"
  git -C "$REPO" checkout -q -b tmp rc
  run resolve_release_base_branch "$REPO" tmp main "$YAML"
  assert_output "release"
}

@test "ancestor branch with prereleases disabled resolves to default branch" {
  prereleases_off
  git -C "$REPO" tag rc
  git -C "$REPO" checkout -q main
  git -C "$REPO" commit -q --allow-empty -m "newer"
  git -C "$REPO" checkout -q -b tmp rc
  run resolve_release_base_branch "$REPO" tmp main "$YAML"
  assert_output "main"
}

@test "RELEASE_BASE_BRANCH override wins" {
  prereleases_on
  git -C "$REPO" checkout -q -b feature
  RELEASE_BASE_BRANCH=custom run resolve_release_base_branch "$REPO" feature main "$YAML"
  assert_output "custom"
}
