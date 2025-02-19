#!/usr/bin/env bash

# Installs `mise` if it isn't already found in PATH
ensure_mise_installed() {
  if ! command -v mise >/dev/null; then
    # Install mise
    gpg --keyserver hkps://keyserver.ubuntu.com --recv-keys 0x7413A06D
    curl https://mise.jdx.dev/install.sh.sig | gpg --decrypt >/tmp/mise-install.sh
    # ensure the above is signed with the mise release key
    MISE_INSTALL_PATH=/usr/local/bin/mise sh /tmp/mise-install.sh

    export MISE_OVERRIDE_TOOL_VERSIONS_FILENAMES=none
    export PATH="$PATH:$HOME/.local/bin"

    # shellcheck disable=SC2016
    # Why: Appending a PATH to BASH_ENV
    {
      echo 'export MISE_OVERRIDE_TOOL_VERSIONS_FILENAMES=none'
      echo 'export PATH="$PATH:$HOME/.local/bin"'
      echo 'eval "$(mise activate bash --shims)"'
    } >>"$BASH_ENV"

    eval "$(mise activate bash --shims)"
  fi
}
