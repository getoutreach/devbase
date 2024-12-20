#!/usr/bin/env bats

bats_load_library "bats-support/load.bash"
bats_load_library "bats-assert/load.bash"

load bootstrap.sh

setup() {
  # This points us to use a temp file for a git repo to operate on, as
  # opposed to the real one.
  REPOPATH=$(mktemp -p "$TMPDIR" -d devbase.bootstrapXXXXXXXXXX)

  git init --initial-branch=main "$REPOPATH"
  cd "$REPOPATH" || exit 1
  git config user.email "bootstrap@devbase.test"
  git commit --allow-empty --message "Initial commit"
  git commit --allow-empty --message "Second commit"
  git switch --create username/feat/feature-branch
  git commit --allow-empty --message "feat: empty branch commit"
}

teardown() {
  rm -rf "$REPOPATH"
}

@test "get_app_version without a version tag in git returns the fallback version" {
  run get_app_version
  assert_output "v0.0.0-dev"
}

@test "get_app_version returns the latest version tag" {
  git switch main
  git tag v1.0.0
  run get_app_version
  assert_output "v1.0.0"

  git commit --allow-empty --message "Third commit"
  git tag v1.1.0
  run get_app_version
  assert_output "v1.1.0"
}
