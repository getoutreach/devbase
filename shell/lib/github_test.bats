#!/usr/bin/env bats

load github.sh

bats_load_library "bats-support/load.bash"
bats_load_library "bats-assert/load.bash"

setup() {
  INSTALL_DIR="$(mktemp -d)"
}

teardown() {
  rm -rf "${INSTALL_DIR}"
}

@test "install_latest_github_release should be able to download and install the latest release of a repo" {
  install_latest_github_release getoutreach/stencil false stencil "${INSTALL_DIR}"

  # We expect the stencil binary to be installed in the install dir.
  assert [ -e "${INSTALL_DIR}/stencil" ]

  run "$INSTALL_DIR/stencil" --version
}

@test "install_latest_github_release should be able to download and install the latest pre-release of a repo" {
  install_latest_github_release getoutreach/stencil true stencil "${INSTALL_DIR}"

  # We expect the stencil binary to be installed in the install dir.
  assert [ -e "${INSTALL_DIR}/stencil" ]

  run "$INSTALL_DIR/stencil" --version
  assert_output --regexp "(rc|unstable)"
}
