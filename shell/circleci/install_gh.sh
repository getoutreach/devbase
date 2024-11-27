#!/usr/bin/env bash
#
# Installs the GitHub CLI (`gh`) if it is not already installed.

set -e

# ARCH is the current architecture of the machine. Valid values are:
#   - amd64
#   - arm64
ARCH="amd64"
if [[ "$(uname -m)" == "aarch64" ]]; then
  ARCH="arm64"
fi

# GH_VERSION is the version of gh to install.
export GH_VERSION=2.62.0

if ! command -v gh >/dev/null; then
  echo "Installing gh"

  wget -O gh.deb https://github.com/cli/cli/releases/download/v${GH_VERSION}/gh_${GH_VERSION}_linux_${ARCH}.deb
  sudo apt-get install --assume-yes --fix-broken ./gh.deb
  rm ./gh.deb
fi
