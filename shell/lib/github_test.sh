#!/usr/bin/env bash
# Tests github.sh functionality.

# No -e because we want to handle errors to make them
# obvious.
set -uo pipefail

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"

# shellcheck source=github.sh
source "${DIR}/github.sh"

# shellcheck source=asdf.sh
source "${DIR}/asdf.sh"

# shellcheck source=logging.sh
source "${DIR}/logging.sh"

test_install_latest_github_release() {
  installDir="$(mktemp -d)"
  trap 'rm -rf "${installDir}"' EXIT

  echo "Should be able to download the latest release of a repo"
  install_latest_github_release getoutreach/stencil false stencil "${installDir}"

  if [[ ! -e "${installDir}/stencil" ]]; then
    echo "Expected stencil to be installed in ${installDir}" >&2
    return 1
  fi

  if ! "${installDir}/stencil" --version; then
    echo "Expected stencil to be executable" >&2
    return 1
  fi
}

test_install_latest_github_release_pre_release() {
  installDir="$(mktemp -d)"
  trap 'rm -rf "${installDir}"' EXIT

  echo "Should be able to download the latest pre-release of a repo"
  install_latest_github_release getoutreach/stencil true stencil "${installDir}"

  if [[ ! -e "${installDir}/stencil" ]]; then
    echo "Expected stencil to be installed in ${installDir}" >&2
    return 1
  fi

  if ! "${installDir}/stencil" --version >/dev/null; then
    echo "Expected stencil to be executable" >&2
    return 1
  fi

  if ! "${installDir}/stencil" --version | grep -qE "(rc|unstable)"; then
    echo "Expected stencil to be a pre-release" >&2
    return 1
  fi
}

test_install_latest_github_release || exit 1
test_install_latest_github_release_pre_release || exit 1
