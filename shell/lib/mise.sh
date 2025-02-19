#!/usr/bin/env bash
#
# mise related functions. Assumes logging.sh is sourced.

# Installs `mise` if it isn't already found in PATH
ensure_mise_installed() {
  if ! command -v mise >/dev/null; then
    # Install mise
    gpg --keyserver hkps://keyserver.ubuntu.com --recv-keys 0x7413A06D
    curl https://mise.jdx.dev/install.sh.sig | gpg --decrypt >/tmp/mise-install.sh
    # ensure the above is signed with the mise release key
    sh /tmp/mise-install.sh
    sudo mv ~/.local/bin/mise /usr/local/bin/

    export MISE_OVERRIDE_TOOL_VERSIONS_FILENAMES=none

    # shellcheck disable=SC2016
    # Why: Appending a PATH to BASH_ENV
    {
      echo 'export MISE_OVERRIDE_TOOL_VERSIONS_FILENAMES=none'
      echo 'eval "$(mise activate bash --shims)"'
    } >>"$BASH_ENV"

    eval "$(mise activate bash --shims)"
  fi
}

install_tool_with_mise() {
  local tool
  local name="$1"
  local version="$2"

  if [[ -n $version ]]; then
    tool="$name@$version"
  else
    tool="$name"
  fi

  ensure_mise_installed

  if ! mise use --global "$tool"; then
    fatal "Error: failed to install $tool via mise" >&2
  fi
}
