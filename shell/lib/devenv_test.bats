#!/usr/bin/env bats
# shellcheck disable=SC2030,SC2031 # Why: each test sets its own env, run in its own subshell.

load logging.sh
load github.sh
load devenv.sh
load test_helper.sh

bats_load_library "bats-support/load.bash"
bats_load_library "bats-assert/load.bash"

setup() {
  CALLS_FILE="$BATS_TEST_TMPDIR/calls"
  : >"$CALLS_FILE"
  export CALLS_FILE
  unset DEVENV_CI_VERSION DEVENV_PRE_RELEASE

  # These stubs replace the github.sh and mise.sh helpers. They record each
  # call and make no network requests.
  # shellcheck disable=SC2329 # Why: called indirectly as a stub.
  bootstrap_github_token() { :; }
  # shellcheck disable=SC2329 # Why: called indirectly as a stub.
  install_tool_with_mise() { echo "install_tool_with_mise $*" >>"$CALLS_FILE"; }
  # shellcheck disable=SC2329 # Why: called indirectly as a stub.
  install_latest_github_release() { echo "install_latest_github_release $*" >>"$CALLS_FILE"; }
}

@test "install_devenv_for_ci installs the latest stable release when nothing is set" {
  run install_devenv_for_ci
  assert_success
  assert_output --partial "Using the latest stable devenv release"

  run cat "$CALLS_FILE"
  assert_output "install_latest_github_release getoutreach/devenv false"
}

@test "install_devenv_for_ci treats an empty DEVENV_CI_VERSION as unset" {
  export DEVENV_CI_VERSION=""
  run install_devenv_for_ci
  assert_success

  run cat "$CALLS_FILE"
  assert_output "install_latest_github_release getoutreach/devenv false"
}

@test "install_devenv_for_ci installs the version in DEVENV_CI_VERSION" {
  export DEVENV_CI_VERSION="1.68.0"
  run install_devenv_for_ci
  assert_success
  assert_output --partial "Using devenv 1.68.0 (from DEVENV_CI_VERSION)"

  run cat "$CALLS_FILE"
  assert_output "install_tool_with_mise github:getoutreach/devenv 1.68.0"
}

@test "install_devenv_for_ci strips a leading v from DEVENV_CI_VERSION" {
  export DEVENV_CI_VERSION="v1.68.0"
  run install_devenv_for_ci
  assert_success

  run cat "$CALLS_FILE"
  assert_output "install_tool_with_mise github:getoutreach/devenv 1.68.0"
}

@test "install_devenv_for_ci prefers DEVENV_CI_VERSION over DEVENV_PRE_RELEASE" {
  export DEVENV_CI_VERSION="1.68.0"
  export DEVENV_PRE_RELEASE="true"
  run install_devenv_for_ci
  assert_success
  assert_output --partial "Ignoring DEVENV_PRE_RELEASE because DEVENV_CI_VERSION is set"

  run cat "$CALLS_FILE"
  assert_output "install_tool_with_mise github:getoutreach/devenv 1.68.0"
}

@test "install_devenv_for_ci installs the latest pre-release when DEVENV_PRE_RELEASE is true" {
  export DEVENV_PRE_RELEASE="true"
  run install_devenv_for_ci
  assert_success
  assert_output --partial "Using the latest devenv pre-release"

  run cat "$CALLS_FILE"
  assert_output "install_latest_github_release getoutreach/devenv true"
}

@test "install_devenv_for_ci fails when the latest release can't be installed" {
  # shellcheck disable=SC2329 # Why: called indirectly as a stub.
  install_latest_github_release() { return 1; }
  run install_devenv_for_ci
  assert_failure
}

@test "install_devenv_for_ci logs an error when the latest release can't be determined" {
  source "$BATS_TEST_DIRNAME/github.sh"
  # shellcheck disable=SC2329 # Why: called indirectly as a stub.
  latest_github_release_version() { :; }
  run install_devenv_for_ci
  assert_failure
  assert_output --partial "Failed to determine version for getoutreach/devenv"
}

@test "install_devenv_for_ci installs even when devenv is already on PATH" {
  mkdir -p "$BATS_TEST_TMPDIR/bin"
  printf '#!/usr/bin/env bash\necho 1.0.0\n' >"$BATS_TEST_TMPDIR/bin/devenv"
  chmod +x "$BATS_TEST_TMPDIR/bin/devenv"
  PATH="$BATS_TEST_TMPDIR/bin:$PATH" run install_devenv_for_ci
  assert_success

  run cat "$CALLS_FILE"
  assert_output "install_latest_github_release getoutreach/devenv false"
}

@test "install_devenv_for_ci trims whitespace and an uppercase V from DEVENV_CI_VERSION" {
  export DEVENV_CI_VERSION="  V1.68.0 "
  run install_devenv_for_ci
  assert_success

  run cat "$CALLS_FILE"
  assert_output "install_tool_with_mise github:getoutreach/devenv 1.68.0"
}

@test "install_devenv_for_ci rejects a DEVENV_CI_VERSION that isn't a version" {
  export DEVENV_CI_VERSION="latest"
  run install_devenv_for_ci
  assert_failure
  assert_output --partial "DEVENV_CI_VERSION must be a devenv release version"

  run cat "$CALLS_FILE"
  assert_output ""
}
