#!/usr/bin/env bats
# shellcheck disable=SC2030,SC2031 # Why: each test sets its own env, run in its own subshell.

load logging.sh
load github.sh
load version.sh
load devenv.sh
load test_helper.sh

bats_load_library "bats-support/load.bash"
bats_load_library "bats-assert/load.bash"

setup() {
  setup_command_stubs
  unset DEVENV_CI_VERSION DEVENV_PRE_RELEASE

  # Replace the github.sh and mise.sh helpers so that no test makes a
  # network request.
  stub_function bootstrap_github_token
  stub_function install_tool_with_mise
  stub_function install_latest_github_release
}

teardown() {
  teardown_command_stubs
}

@test "install_devenv_for_ci installs the latest stable release when nothing is set" {
  run install_devenv_for_ci
  assert_success
  assert_output --partial "Using the latest stable devenv release"

  assert_stub_called install_latest_github_release getoutreach/devenv false
  assert_stub_not_called install_tool_with_mise
}

@test "install_devenv_for_ci treats an empty DEVENV_CI_VERSION as unset" {
  export DEVENV_CI_VERSION=""
  run install_devenv_for_ci
  assert_success

  assert_stub_called install_latest_github_release getoutreach/devenv false
  assert_stub_not_called install_tool_with_mise
}

@test "install_devenv_for_ci installs the version in DEVENV_CI_VERSION" {
  export DEVENV_CI_VERSION="1.68.0"
  run install_devenv_for_ci
  assert_success
  assert_output --partial "Using devenv 1.68.0 (from DEVENV_CI_VERSION)"

  assert_stub_called install_tool_with_mise github:getoutreach/devenv 1.68.0
  assert_stub_not_called install_latest_github_release
}

@test "install_devenv_for_ci strips a leading v from DEVENV_CI_VERSION" {
  export DEVENV_CI_VERSION="v1.68.0"
  run install_devenv_for_ci
  assert_success

  assert_stub_called install_tool_with_mise github:getoutreach/devenv 1.68.0
  assert_stub_not_called install_latest_github_release
}

@test "install_devenv_for_ci prefers DEVENV_CI_VERSION over DEVENV_PRE_RELEASE" {
  export DEVENV_CI_VERSION="1.68.0"
  export DEVENV_PRE_RELEASE="true"
  run install_devenv_for_ci
  assert_success
  assert_output --partial "Ignoring DEVENV_PRE_RELEASE because DEVENV_CI_VERSION is set"

  assert_stub_called install_tool_with_mise github:getoutreach/devenv 1.68.0
  assert_stub_not_called install_latest_github_release
}

@test "install_devenv_for_ci installs the latest pre-release when DEVENV_PRE_RELEASE is true" {
  export DEVENV_PRE_RELEASE="true"
  run install_devenv_for_ci
  assert_success
  assert_output --partial "Using the latest devenv pre-release"

  assert_stub_called install_latest_github_release getoutreach/devenv true
  assert_stub_not_called install_tool_with_mise
}

@test "install_devenv_for_ci fails when the latest release can't be installed" {
  stub_function install_latest_github_release 1
  run install_devenv_for_ci
  assert_failure
}

@test "install_devenv_for_ci trims whitespace and an uppercase V from DEVENV_CI_VERSION" {
  export DEVENV_CI_VERSION="  V1.68.0 "
  run install_devenv_for_ci
  assert_success

  assert_stub_called install_tool_with_mise github:getoutreach/devenv 1.68.0
}

@test "install_devenv_for_ci rejects a DEVENV_CI_VERSION that isn't a version" {
  export DEVENV_CI_VERSION="latest"
  run install_devenv_for_ci
  assert_failure
  assert_output --partial "DEVENV_CI_VERSION must be a devenv release version"

  assert_stub_not_called install_tool_with_mise
  assert_stub_not_called install_latest_github_release
}
