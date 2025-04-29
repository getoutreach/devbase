#!/usr/bin/env bash
#
# mise related functions. Assumes logging.sh is sourced.

# Installs `mise` if it isn't already found in PATH.
# If running as root, install to /usr/local/bin. Otherwise, install
# to $HOME/.local/bin.
ensure_mise_installed() {
  if ! command -v mise >/dev/null; then
    local is_root
    # From: https://askubuntu.com/a/15856
    if [[ $EUID -eq 0 ]]; then
      is_root=true
    else
      is_root=
    fi

    if [[ -n $is_root ]]; then
      export MISE_INSTALL_PATH=/usr/local/bin/mise
    fi

    # Install mise
    gpg --keyserver hkps://keyserver.ubuntu.com --recv-keys 0x7413A06D
    curl https://mise.jdx.dev/install.sh.sig | gpg --decrypt >/tmp/mise-install.sh
    # ensure the above is signed with the mise release key
    sh /tmp/mise-install.sh

    unset MISE_INSTALL_PATH

    local mise_manages_tool_versions="${ALLOW_MISE_TO_MANAGE_TOOL_VERSIONS:-}"

    # shellcheck disable=SC2016
    # Why: Appending a PATH to BASH_ENV
    {
      if [[ -z $is_root ]]; then
        echo 'export PATH="$HOME/.local/bin:$PATH"'
      fi
      if [[ -z $mise_manages_tool_versions ]]; then
        echo 'export MISE_OVERRIDE_TOOL_VERSIONS_FILENAMES=none'
      fi
      echo 'eval "$(mise activate bash --shims)"'
    } >>"$BASH_ENV"

    if [[ -z $is_root ]]; then
      export PATH="$HOME/.local/bin:$PATH"
    fi
    if [[ -z $mise_manages_tool_versions ]]; then
      # Let asdf manage .tool-versions for now
      export MISE_OVERRIDE_TOOL_VERSIONS_FILENAMES=none
    fi
    eval "$(mise activate bash --shims)"
  fi
}

# install_tool_with_mise tool [version]
#
# Installs a tool using mise. If the version is not specified, the latest
# version will be installed as detected by mise. Please note that if
# you specify a version, it will override any tool-specific config
# (e.g., exe for the ubi backend) that you may already have set.
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
#
# Sets one or more mise config keys globally.
mise_tool_config_set() {
  ensure_mise_installed

  local config_file="$HOME/.config/mise/config.toml"

  # A config file is required to set the config.
  if [[ ! -f $config_file ]]; then
    mkdir -p "$(dirname "$config_file")"
    touch "$config_file"
  fi

  local slug="$1"
  shift
  while [[ $# -gt 0 ]]; do
    local key="$1"
    shift
    local value="$1"
    shift
    mise config set --file="$config_file" "tools.$slug.$key" "$value"
  done
}
