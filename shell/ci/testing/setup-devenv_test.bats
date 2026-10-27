#!/usr/bin/env bats
# shellcheck disable=SC2030,SC2031 # Why: each test sets its own env, run in its own subshell.

load ../../lib/test_helper.sh

bats_load_library "bats-support/load.bash"
bats_load_library "bats-assert/load.bash"

# These tests run setup-devenv.sh as a subprocess with every external
# tool stubbed, and check that it installs devenv in CI. Provisioning
# and the E2E runner stay off, so the script never reaches `exec`.
setup() {
  setup_command_stubs
  stub_command mise ""
  stub_command gh "v1.68.0"
  stub_command git ""
  stub_command kubectl ""
  stub_command kubecfg ""

  HOME="$(mktempdir setup-devenv-home-XXXXXX)"
  export HOME
  export CI=true VAULT_ROLE_ID=test GITHUB_TOKEN=test-token
  export PROVISION=false E2E=false
  unset DEVENV_CI_VERSION DEVENV_PRE_RELEASE DATADOG_API_KEY BASH_ENV
}

teardown() {
  rm -rf "$HOME"
  teardown_command_stubs
}

@test "setup-devenv.sh installs the latest stable devenv in CI" {
  run bash "$BATS_TEST_DIRNAME/setup-devenv.sh"
  assert_success

  run grep -F "mise use --global github:getoutreach/devenv@1.68.0" "$STUB_CALLS_FILE"
  assert_success
  run stub_argv gh
  assert_line "--exclude-pre-releases"
}

@test "setup-devenv.sh installs the DEVENV_CI_VERSION devenv in CI" {
  export DEVENV_CI_VERSION="1.67.0"
  run bash "$BATS_TEST_DIRNAME/setup-devenv.sh"
  assert_success

  run grep -F "mise use --global github:getoutreach/devenv@1.67.0" "$STUB_CALLS_FILE"
  assert_success
  assert_stub_not_called gh
}

@test "setup-devenv.sh doesn't install devenv outside CI" {
  unset CI
  run bash "$BATS_TEST_DIRNAME/setup-devenv.sh"
  assert_success

  run grep -c "^mise use" "$STUB_CALLS_FILE"
  assert_output "0"
}
