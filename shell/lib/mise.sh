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

    # shellcheck disable=SC2016
    # Why: Appending a PATH to BASH_ENV
    {
      echo 'export MISE_OVERRIDE_TOOL_VERSIONS_FILENAMES=none'
      echo 'eval "$(mise activate bash --shims)"'
    } >>"$BASH_ENV"

    # Let asdf manage .tool-versions for now
    export MISE_OVERRIDE_TOOL_VERSIONS_FILENAMES=none
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

  info "Installing $tool via mise"

  if ! mise use --global "$tool"; then
    fatal "Error: failed to install $tool via mise" >&2
  fi
}

# mise_tool_config_set slug key value [...key value...]
mise_tool_config_set() {
  local slug="$1"
  shift
  while [[ $# -gt 0 ]]; do
    local key="$1"
    shift
    local value="$1"
    shift
    MISE_VERBOSE=1 mise config set "tools.$slug.$key" "$value"
  done
}
