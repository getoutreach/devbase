#!/usr/bin/env bash
#
# Installs the GitHub CLI (`gh`) if it is not already installed.

set -e

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
LIB_DIR="${DIR}/../lib"

# GH_VERSION is the version of gh to install.
export GH_VERSION=2.62.0

# shellcheck source=../lib/mise.sh
source "$LIB_DIR/mise.sh"

ensure_mise_installed

mise use --global "gh@$GH_VERSION"
