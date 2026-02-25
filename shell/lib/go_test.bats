#!/usr/bin/env bash

bats_load_library "bats-support/load.bash"
bats_load_library "bats-assert/load.bash"

load go.sh
load test_helper.sh

setup() {
  # This points us to use a temp file for a git repo to operate on, as
  # opposed to the real one.
  REPOPATH=$(mktempdir devbase-lib-go-XXXXXX)

  git init --initial-branch=main "$REPOPATH"
  cd "$REPOPATH" || exit 1
  git config user.name "Test User"
  git config user.email "testuser@example.com"
}

teardown() {
  rm -rf "$REPOPATH"
}

@test "go_mod_dirs finds all go.mod files in the repo" {
  mkdir -p moduleA moduleB/submodule
  touch moduleA/go.mod
  touch moduleB/submodule/go.mod
  touch go.mod
  git add .

  run go_mod_dirs
  assert_success
  assert_output ". moduleA moduleB/submodule"
}

@test "go_mod_dirs excludes go.mod files in IGNORED_GO_MOD_DIRS" {
  mkdir -p moduleA moduleB/submodule moduleC
  touch moduleA/go.mod
  touch moduleB/submodule/go.mod
  touch moduleC/go.mod
  touch go.mod

  IGNORED_GO_MOD_DIRS="moduleB/submodule moduleC" run go_mod_dirs
  assert_success
  assert_output ". moduleA" # output is sorted
}
