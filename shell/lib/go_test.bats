#!/usr/bin/env bash

bats_load_library "bats-support/load.bash"
bats_load_library "bats-assert/load.bash"

load go.sh
load version.sh
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
  git add .

  IGNORED_GO_MOD_DIRS="moduleB/submodule moduleC" run go_mod_dirs
  assert_success
  assert_output ". moduleA" # output is sorted
}

@test "golangci_lint_built_with_go reads the Go version from golangci-lint --version" {
  run golangci_lint_built_with_go "golangci-lint has version 2.9.0 built with go1.26.0 from abc1234 on 2026-02-10T18:00:00Z"
  assert_success
  assert_output "1.26.0"
}

@test "golangci_lint_built_with_go reads a Go version without a patch" {
  run golangci_lint_built_with_go "golangci-lint has version 2.13.0 built with go1.27 from abc1234 on 2026-08-12T18:00:00Z"
  assert_success
  assert_output "1.27"
}

@test "golangci_lint_built_with_go drops the prerelease suffix of a Go version" {
  run golangci_lint_built_with_go "golangci-lint has version 2.13.0 built with go1.27rc1 from abc1234 on 2026-08-12T18:00:00Z"
  assert_success
  assert_output "1.27"
}

@test "golangci_lint_built_with_go prints nothing for a development build of Go" {
  run golangci_lint_built_with_go "golangci-lint has version 2.13.0 built with devel go1.28-abc1234 from abc1234 on 2026-08-12T18:00:00Z"
  assert_success
  assert_output ""
}

@test "golangci_lint_built_with_go prints nothing for unexpected output" {
  run golangci_lint_built_with_go "unexpected"
  assert_success
  assert_output ""
}

@test "golangci_lint_supports_go succeeds when golangci-lint was built with the same Go" {
  run golangci_lint_supports_go "1.26.0" "1.26.0"
  assert_success
}

@test "golangci_lint_supports_go ignores the patch version of both" {
  run golangci_lint_supports_go "1.26.3" "1.26.0"
  assert_success
}

@test "golangci_lint_supports_go succeeds when golangci-lint was built with a newer Go" {
  run golangci_lint_supports_go "1.25.7" "1.26.0"
  assert_success
}

@test "golangci_lint_supports_go fails when golangci-lint was built with an older Go" {
  run golangci_lint_supports_go "1.26.0" "1.25.7"
  assert_failure
}

@test "golangci_lint_supports_go compares minor versions as numbers" {
  run golangci_lint_supports_go "1.10.0" "1.9.0"
  assert_failure
}

@test "golangci_lint_supports_go fails for an older major version" {
  run golangci_lint_supports_go "2.0.0" "1.99.0"
  assert_failure
}

@test "golangci_lint_supports_go ignores a prerelease suffix of the toolchain" {
  run golangci_lint_supports_go "1.26rc1" "1.26.0"
  assert_success
  assert_output ""
}

@test "golangci_lint_supports_go fails for a prerelease of a newer minor version" {
  run golangci_lint_supports_go "1.27rc1" "1.26.3"
  assert_failure 1
  assert_output ""
}

@test "golangci_lint_supports_go returns 2 for a development toolchain" {
  run golangci_lint_supports_go "devel" "1.26.3"
  assert_failure 2
  assert_output ""
}

@test "golangci_lint_supports_go returns 2 for an empty toolchain version" {
  run golangci_lint_supports_go "" "1.26.3"
  assert_failure 2
  assert_output ""
}
