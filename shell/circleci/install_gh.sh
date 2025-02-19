#!/usr/bin/env bash
#
# Installs the GitHub CLI (`gh`) if it is not already installed.

set -e

# GH_VERSION is the version of gh to install.
export GH_VERSION=2.62.0

if ! command -v mise >/dev/null; then
  # Install mise
  gpg --keyserver hkps://keyserver.ubuntu.com --recv-keys 0x7413A06D
  curl https://mise.jdx.dev/install.sh.sig | gpg --decrypt >/tmp/mise-install.sh
  # ensure the above is signed with the mise release key
  sh /tmp/mise-install.sh

  export MISE_OVERRIDE_TOOL_VERSIONS_FILENAMES=none
  export PATH="$PATH:$HOME/.local/bin"

  # shellcheck disable=SC2016
  # Why: Appending a shell command to .bashrc
  echo 'eval "$(mise activate bash --shims)"' >>~/.bashrc
  eval "$(mise activate bash --shims)"
fi

mise use --global "gh@$GH_VERSION"
