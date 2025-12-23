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
  echo '[tools]' >>"$MISE_GLOBAL_CONFIG_FILE"
  echo 'github-cli = "latest"' >>"$MISE_GLOBAL_CONFIG_FILE"
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
}

@test "separate mise install" {
  run mise doctor
  assert_output --partial "config: $MISE_CONFIG_DIR"
}

@test "install_latest_github_release should be able to download and install the latest release of a repo" {
  if circleci_pr_is_fork; then
    skip "Skipping test in fork PR, no GitHub token available to utilize gh."
  fi
  install_latest_github_release getoutreach/stencil false stencil

  # We expect the stencil binary to be installed in the install dir.
  assert mise which stencil

  run "$(mise which stencil)" --version
}

@test "install_latest_github_release should be able to download and install the latest pre-release of a repo" {
  if circleci_pr_is_fork; then
    skip "Skipping test in fork PR, no GitHub token available to utilize gh."
  fi
  install_latest_github_release getoutreach/stencil true stencil

  # We expect the stencil binary to be installed in the install dir.
  assert mise which stencil

  run "$(mise which stencil)" --version
  assert_output --regexp "(rc|unstable)"
}
