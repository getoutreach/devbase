#!/usr/bin/env bash

bats_load_library "bats-support/load.bash"
bats_load_library "bats-assert/load.bash"

load bootstrap.sh
load test_helper.sh

setup() {
  # This points us to use a temp file for a git repo to operate on, as
  # opposed to the real one.
  REPOPATH=$(mktempdir devbase.bootstrapXXXXXXXXXX)

  git init --initial-branch=main "$REPOPATH"
  cd "$REPOPATH" || exit 1
  git config user.name "Test User"
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

@test "get_app_version with VERSIONING_SCHEME=sha returns the commit hash" {
  VERSIONING_SCHEME=sha run get_app_version
  assert_output "$(git rev-parse HEAD)"
}

@test "get_app_version returns the latest version tag" {
  git switch main
  git tag --message "1.0.0" v1.0.0
  run get_app_version
  assert_output "v1.0.0"

  git commit --allow-empty --message "Third commit"
  git tag --message "1.1.0" v1.1.0
  run get_app_version
  assert_output "v1.1.0"

  VERSIONING_SCHEME=sha run get_app_version
  assert_output "$(git rev-parse HEAD)"
}

@test "stencil_version reads the Stencil version from stencil.lock" {
  cat >"$REPOPATH"/stencil.lock <<EOF
version: v1.66.6-rc.6
EOF
  run stencil_version
  assert_output "v1.66.6-rc.6"
}

@test "stencil_arg reads a nested argument from service.yaml" {
  cat >"$REPOPATH"/service.yaml <<EOF
arguments:
  foo:
    bar: baz
EOF
  run stencil_arg foo.bar
  assert_output "baz"
}

@test "stencil_module_version returns the version from stencil.lock" {
  # Create a mock stencil.lock file
  cat >"$REPOPATH"/stencil.lock <<EOF
modules:
  - name: example.com/module
    version: v1.2.3
  - name: github.com/getoutreach/devbase
    version: v2.3.4
EOF

  run stencil_module_version example.com/module
  assert_output "v1.2.3"

  run stencil_module_version github.com/getoutreach/devbase
  assert_output "v2.3.4"

  # Test with a non-existent module
  run stencil_module_version github.com/nonexistent/module
  assert_output ""
}

@test "managed_by_stencil succeeds for a file managed by stencil" {
  # Create a mock stencil.lock file
  cat >"$REPOPATH"/stencil.lock <<EOF
files:
  - name: exists.txt
    template: exists.txt.tpl
    module: example.com/module
EOF
  run managed_by_stencil exists.txt
  assert_success
}

@test "managed_by_stencil fails for a file not managed by stencil" {
  # Create a mock stencil.lock file
  cat >"$REPOPATH"/stencil.lock <<EOF
files:
  - name: exists.txt
    template: exists.txt.tpl
    module: example.com/module
EOF
  run managed_by_stencil nonexistent.txt
  assert_failure
}
