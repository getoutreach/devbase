#!/usr/bin/env bats

load circleci.sh
load github.sh
load test_helper.sh

bats_load_library "bats-support/load.bash"
bats_load_library "bats-assert/load.bash"

setup() {
  local MISE_CONFIG_DIR MISE_DATA_DIR MISE_STATE_DIR

  MISE_CONFIG_DIR="$(mktempdir mise-config-XXXXXX)"
  MISE_DATA_DIR="$(mktempdir mise-data-XXXXXX)"
  MISE_STATE_DIR="$(mktempdir mise-state-XXXXXX)"
  export MISE_CONFIG_DIR MISE_DATA_DIR MISE_STATE_DIR

  export MISE_GLOBAL_CONFIG_ROOT="$MISE_CONFIG_DIR"
  export MISE_GLOBAL_CONFIG_FILE="$MISE_CONFIG_DIR/global.toml"
  export MISE_OVERRIDE_CONFIG_FILENAMES="global.toml"
  {
    echo '[tools]'
    echo 'github-cli = "latest"'
    echo 'wait-for-gh-rate-limit = "1.1.1"'
  } >>"$MISE_GLOBAL_CONFIG_FILE"

  setup_command_stubs
}

teardown() {
  rm -rf "$MISE_CONFIG_DIR"
  rm -rf "$MISE_DATA_DIR"
  rm -rf "$MISE_STATE_DIR"
  unset MISE_CONFIG_DIR
  unset MISE_DATA_DIR
  unset MISE_STATE_DIR
  unset MISE_GLOBAL_CONFIG_ROOT
  unset MISE_GLOBAL_CONFIG_FILE
  unset MISE_OVERRIDE_CONFIG_FILENAMES

  teardown_command_stubs
}

@test "separate mise install" {
  run mise doctor
  assert_output --partial "config: $MISE_CONFIG_DIR"
}

@test "latest_github_release_version asks gh to exclude pre-releases when they are not wanted" {
  stub_command gh "v1.45.0"

  run latest_github_release_version getoutreach/stencil false
  assert_success
  assert_output "v1.45.0"

  run stub_argv gh
  assert_line "getoutreach/stencil"
  assert_line "--exclude-drafts"
  assert_line "--exclude-pre-releases"
}

@test "latest_github_release_version takes the newest release of any kind when pre-releases are wanted" {
  stub_command gh "v1.45.0"

  run latest_github_release_version getoutreach/stencil true
  assert_success
  assert_output "v1.45.0"

  run stub_argv gh
  assert_line "--exclude-drafts"
  refute_line "--exclude-pre-releases"
}

@test "install_latest_github_release fails when there is no release to install" {
  stub_command gh ""

  run install_latest_github_release getoutreach/stencil false stencil
  assert_failure
  assert_output --partial "Failed to determine version for getoutreach/stencil"
}

@test "install_latest_github_release should be able to download and install the latest release of a repo" {
  if circleci_pr_is_fork; then
    skip "Skipping test in fork PR, no GitHub token available to utilize gh."
  fi
  install_latest_github_release getoutreach/stencil false stencil

  # We expect the stencil binary to be installed in the install dir.
  assert mise which stencil

  run "$(mise which stencil)" --version
  assert_success
}

@test "install_latest_github_release should be able to download and install the latest pre-release of a repo" {
  if circleci_pr_is_fork; then
    skip "Skipping test in fork PR, no GitHub token available to utilize gh."
  fi

  # The newest release is the stable one between a release and the next
  # commit to the repo, so a pre-release is not always available.
  local tag
  tag="$(latest_github_release_version getoutreach/stencil true)"
  if [[ ! $tag =~ (rc|unstable) ]]; then
    skip "Newest getoutreach/stencil release ($tag) is stable, not a pre-release."
  fi

  install_latest_github_release getoutreach/stencil true stencil

  # We expect the stencil binary to be installed in the install dir.
  assert mise which stencil

  run "$(mise which stencil)" --version
  assert_success
  assert_output --regexp "(rc|unstable)"
}
