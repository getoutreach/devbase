#!/usr/bin/env bash

bats_load_library "bats-support/load.bash"
bats_load_library "bats-assert/load.bash"

load go.sh
load mise.sh
load sed.sh
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

@test "golangci_lint_built_with_go prints only the first golangci-lint line" {
  run golangci_lint_built_with_go "$(printf 'noise built with go9.9.9\ngolangci-lint has version 2.9.0 built with go1.26.0 from abc1234\ngolangci-lint has version 2.0.0 built with go1.24.0 from abc1234')"
  assert_success
  assert_output "1.26.0"
}

@test "golangci_lint_built_with_go ignores text that is not from golangci-lint" {
  run golangci_lint_built_with_go "mise: built with go1.26.0"
  assert_success
  assert_output ""
}

# stub_stencil_environment stubs the functions from bootstrap.sh and
# logging.sh that the Go directive checks need. The test sets STENCIL_GO_STANZA
# to the value of the go.stanza argument in service.yaml, and
# STENCIL_MANAGED_FILES to the space-separated files that stencil.lock lists.
stub_stencil_environment() {
  STENCIL_GO_STANZA="null"
  STENCIL_MANAGED_FILES="go.mod"
  get_repo_directory() { echo "$REPOPATH"; }
  stencil_arg() { echo "$STENCIL_GO_STANZA"; }
  managed_by_stencil() { [[ " $STENCIL_MANAGED_FILES " == *" $1 "* ]]; }
  error() { echo "error: $*" >&2; }
  warn() { echo "warn: $*" >&2; }
  info() { echo "info: $*" >&2; }
}

# write_go_mod(directory, go_version) writes a go.mod file.
write_go_mod() {
  mkdir -p "$1"
  printf 'module example.com/test\n\ngo %s\n\nrequire (\n\texample.com/dep v1.0.0\n)\n' "$2" >"$1/go.mod"
}

@test "go_mod_go_version prints the version of the go directive" {
  write_go_mod . 1.25.7

  run go_mod_go_version go.mod
  assert_success
  assert_output "1.25.7"
}

@test "go_mod_go_version ignores a comment after the go directive" {
  printf 'module example.com/test\n\ngo 1.25.7 // keep\n' >go.mod

  run go_mod_go_version go.mod
  assert_success
  assert_output "1.25.7"
}

@test "go_mod_go_version prints nothing without a go directive" {
  printf 'module example.com/test\n' >go.mod

  run go_mod_go_version go.mod
  assert_success
  assert_output ""
}

@test "expected_go_mod_go_version reads the Go version from .tool-versions" {
  stub_stencil_environment
  printf 'nodejs 22.22.0\ngolang 1.25.7\n' >.tool-versions

  run expected_go_mod_go_version
  assert_success
  assert_output "1.25.7"
}

@test "expected_go_mod_go_version prefers the go.stanza argument" {
  stub_stencil_environment
  STENCIL_GO_STANZA="1.19"
  printf 'golang 1.25.7\n' >.tool-versions

  run expected_go_mod_go_version
  assert_success
  assert_output "1.19"
}

@test "expected_go_mod_go_version prints nothing without a Go version" {
  stub_stencil_environment

  run expected_go_mod_go_version # no .tool-versions
  assert_success
  assert_output ""

  printf 'nodejs 22.22.0\n' >.tool-versions
  run expected_go_mod_go_version # no golang entry
  assert_success
  assert_output ""
}

@test "check_go_mod_go_version succeeds when the go directive matches .tool-versions" {
  stub_stencil_environment
  printf 'golang 1.25.7\n' >.tool-versions
  write_go_mod . 1.25.7

  run check_go_mod_go_version .
  assert_success
  assert_output ""
}

@test "check_go_mod_go_version fails when the go directive differs from .tool-versions" {
  stub_stencil_environment
  printf 'golang 1.25.7\n' >.tool-versions
  write_go_mod . 1.26.0

  run check_go_mod_go_version .
  assert_failure 1
  assert_output --partial "The go directive in go.mod is '1.26.0', but Stencil provides Go 1.25.7"
}

@test "check_go_mod_go_version fails when the go directive is missing" {
  stub_stencil_environment
  printf 'golang 1.25.7\n' >.tool-versions
  printf 'module example.com/test\n' >go.mod

  run check_go_mod_go_version .
  assert_failure 1
  assert_output --partial "The go directive in go.mod is 'none'"
}

@test "check_go_mod_go_version compares with the go.stanza argument when it is set" {
  stub_stencil_environment
  STENCIL_GO_STANZA="1.19"
  printf 'golang 1.25.7\n' >.tool-versions
  write_go_mod . 1.19

  run check_go_mod_go_version .
  assert_success
  assert_output ""
}

@test "check_go_mod_go_version skips a go.mod that Stencil does not manage" {
  stub_stencil_environment
  printf 'golang 1.25.7\n' >.tool-versions
  write_go_mod pkg 1.24.0

  run check_go_mod_go_version pkg
  assert_success
  assert_output ""
}

@test "check_go_mod_go_version checks a go.mod in a subdirectory that Stencil manages" {
  stub_stencil_environment
  STENCIL_MANAGED_FILES="go.mod pkg/go.mod"
  printf 'golang 1.25.7\n' >.tool-versions
  write_go_mod pkg 1.24.0

  run check_go_mod_go_version pkg
  assert_failure 1
  assert_output --partial "The go directive in pkg/go.mod is '1.24.0'"
}

@test "check_go_mod_go_version warns and succeeds when no Go version is known" {
  stub_stencil_environment
  write_go_mod . 1.25.7

  run check_go_mod_go_version .
  assert_success
  assert_output --partial "warn: Not checking the go directive of go.mod"
}

@test "go_mod_drop_toolchain removes the toolchain directive and the blank line after it" {
  printf 'module example.com/test\n\ngo 1.25.7\n\ntoolchain go1.25.7\n\nrequire (\n\texample.com/dep v1.0.0\n)\n' >go.mod

  run go_mod_drop_toolchain go.mod
  assert_success
  run cat go.mod
  assert_output "$(printf 'module example.com/test\n\ngo 1.25.7\n\nrequire (\n\texample.com/dep v1.0.0\n)')"
}

@test "go_mod_drop_toolchain removes a toolchain directive that follows the go directive directly" {
  printf 'module example.com/test\n\ngo 1.25.7\ntoolchain go1.25.7\n\nrequire example.com/dep v1.0.0\n' >go.mod

  run go_mod_drop_toolchain go.mod
  assert_success
  run cat go.mod
  assert_output "$(printf 'module example.com/test\n\ngo 1.25.7\n\nrequire example.com/dep v1.0.0')"
}

@test "go_mod_drop_toolchain leaves a go.mod without a toolchain directive as it is" {
  printf 'module example.com/test\n\ngo 1.25.7\n\n\nrequire example.com/dep v1.0.0\n' >go.mod

  run go_mod_drop_toolchain go.mod
  assert_success
  run cat go.mod
  assert_output "$(printf 'module example.com/test\n\ngo 1.25.7\n\n\nrequire example.com/dep v1.0.0')"
}

@test "check_tool_versions_no_golang_override fails when the toolverOverride block sets golang" {
  stub_stencil_environment
  cat >.tool-versions <<'EOF'
## <<Stencil::Block(toolverOverride)>>
golang 1.26.0
## <</Stencil::Block>>
nodejs 22.22.0
golang 1.25.7
## <<Stencil::Block(toolver)>>
## <</Stencil::Block>>
EOF

  run check_tool_versions_no_golang_override
  assert_failure 1
  assert_output --partial "sets golang 1.26.0. Remove it, set versions.go to 1.26.0 in service.yaml"
}

@test "check_tool_versions_no_golang_override ignores golang outside the toolverOverride block" {
  stub_stencil_environment
  cat >.tool-versions <<'EOF'
## <<Stencil::Block(toolverOverride)>>
nodejs 20.0.0
## <</Stencil::Block>>
nodejs 22.22.0
golang 1.25.7
## <<Stencil::Block(toolver)>>
golang 1.24.0
## <</Stencil::Block>>
EOF

  run check_tool_versions_no_golang_override
  assert_success
  assert_output ""
}

@test "check_tool_versions_no_golang_override skips a repository whose go.mod Stencil does not manage" {
  stub_stencil_environment
  STENCIL_MANAGED_FILES=""
  cat >.tool-versions <<'EOF'
## <<Stencil::Block(toolverOverride)>>
golang 1.26.0
## <</Stencil::Block>>
EOF

  run check_tool_versions_no_golang_override
  assert_success
  assert_output ""
}

@test "check_tool_versions_no_golang_override succeeds without a .tool-versions file" {
  stub_stencil_environment

  run check_tool_versions_no_golang_override
  assert_success
  assert_output ""
}

@test "go_mod_file_for_dir prints go.mod for the repository root" {
  run go_mod_file_for_dir .
  assert_success
  assert_output "go.mod"
}

@test "go_mod_file_for_dir prints the go.mod path in a subdirectory" {
  run go_mod_file_for_dir moduleB/submodule
  assert_success
  assert_output "moduleB/submodule/go.mod"
}

# write_root_and_child_go_mod writes a root go.mod and a child go.mod in pkg
# whose go directive, toolchain directive, and gobox version are out of date.
write_root_and_child_go_mod() {
  printf 'module example.com/root\n\ngo 1.25.7\n\nrequire (\n\tgithub.com/getoutreach/gobox v1.120.1\n)\n' >go.mod
  mkdir -p pkg
  printf 'module example.com/root/pkg\n\ngo 1.25.0\n\ntoolchain go1.25.0\n\nrequire (\n\tgithub.com/getoutreach/gobox v1.100.0\n\texample.com/other v1.0.0\n)\n' >pkg/go.mod
}

@test "go_mod_sync_child syncs the go directive, drops the toolchain, and syncs listed modules" {
  stub_stencil_environment
  write_root_and_child_go_mod

  run go_mod_sync_child pkg/go.mod go.mod "example.com/missing github.com/getoutreach/gobox"
  assert_success
  assert_output --partial "warn: Module example.com/missing not found in go.mod, skipping"
  run cat pkg/go.mod
  assert_output "$(printf 'module example.com/root/pkg\n\ngo 1.25.7\n\nrequire (\n\tgithub.com/getoutreach/gobox v1.120.1\n\texample.com/other v1.0.0\n)')"
}

@test "go_mod_sync_child leaves modules that are not listed as they are" {
  stub_stencil_environment
  write_root_and_child_go_mod

  run go_mod_sync_child pkg/go.mod go.mod
  assert_success
  run cat pkg/go.mod
  assert_output --partial "github.com/getoutreach/gobox v1.100.0"
}

@test "go_mod_managed_by_stencil succeeds for the root when stencil.lock lists go.mod" {
  stub_stencil_environment

  run go_mod_managed_by_stencil .
  assert_success
}

@test "go_mod_managed_by_stencil succeeds for a subdirectory that stencil.lock lists" {
  stub_stencil_environment
  STENCIL_MANAGED_FILES="go.mod pkg/go.mod"

  run go_mod_managed_by_stencil pkg
  assert_success
}

@test "go_mod_managed_by_stencil fails for a subdirectory that stencil.lock does not list" {
  stub_stencil_environment

  run go_mod_managed_by_stencil pkg
  assert_failure
}

@test "go_mod_unmanaged_child_dirs prints the unmanaged child modules, and skips the root and managed modules" {
  stub_stencil_environment
  STENCIL_MANAGED_FILES="pkg/go.mod" # the root go.mod is not managed
  mkdir -p pkg other third
  touch go.mod pkg/go.mod other/go.mod third/go.mod

  run go_mod_unmanaged_child_dirs
  assert_success
  assert_output "$(printf 'other\nthird')"
}
