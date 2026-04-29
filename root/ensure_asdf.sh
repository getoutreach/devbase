#!/usr/bin/env bash
# This script ensures that all of the entries in .tool-versions have
# been installed, and if not, that they are installed.
# This is meant to be ran before calling out to mage or other tools.
set -e

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"

# shellcheck source=../shell/lib/circleci.sh
source "$DIR/../shell/lib/circleci.sh"

# When CI is installing the E2E toolset, devbase does not use asdf;
# required tools are installed via mise from mise.e2e.toml. Skip the
# asdf ensure step entirely to avoid spurious "asdf: command not found"
# warnings.
if circleci_should_install_e2e_tools; then
  exit 0
fi

# shellcheck source=../shell/lib/asdf.sh
source "$DIR/../shell/lib/asdf.sh"

asdf_devbase_ensure
