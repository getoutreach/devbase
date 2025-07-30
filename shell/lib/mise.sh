#!/usr/bin/env bash
#
# mise related functions. Assumes logging.sh and shell.sh are sourced.

# Installs `mise` if it isn't already found in PATH.
# If running as root, install to /usr/local/bin. Otherwise, install
# to $HOME/.local/bin.
ensure_mise_installed() {
  local is_root
  # From: https://askubuntu.com/a/15856
  if [[ $EUID -eq 0 ]]; then
    is_root=true
  else
    is_root=
  fi
  local local_bin="$HOME/.local/bin"
  if [[ -z $is_root ]] && ! echo "$PATH" | grep -qF "$local_bin"; then
    export PATH="$HOME/.local/bin:$PATH"
  fi

  if ! command -v mise >/dev/null; then
    if [[ -n $is_root ]]; then
      export MISE_INSTALL_PATH=/usr/local/bin/mise
    fi

    info "Installing mise to ${MISE_INSTALL_PATH:-$HOME/.local/bin/mise}"

    install_mise

    unset MISE_INSTALL_PATH

    local mise_manages_tool_versions="${ALLOW_MISE_TO_MANAGE_TOOL_VERSIONS:-}"

    if [[ -n $BASH_ENV ]]; then
      info_sub "Adding mise to BASH_ENV: $BASH_ENV"
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
    fi

    if [[ -z $mise_manages_tool_versions ]]; then
      # Let asdf manage .tool-versions for now
      export MISE_OVERRIDE_TOOL_VERSIONS_FILENAMES=none
    fi
    info_sub "Activating mise in current shell"
    eval "$(mise activate bash --shims)"
  fi
}

# Installs mise via the official install script (making sure that it is
# signed via GPG). If that fails in CI and the CI worker is running
# Ubuntu, install mise via the official apt repository.
install_mise() {
  local install_script=/tmp/mise-install.sh

  if [[ ! -f $install_script || "$(wc -c "$install_script")" -eq 0 ]]; then
    if ! retry 5 5 gpg --keyserver hkps://keys.openpgp.org --recv-keys 0x24853ec9f655ce80b48e6c3a8b81c9d17413a06d; then
      fatal "Could not import mise GPG release key"
    fi
    # ensure the install script is signed with the mise release key
    if ! retry 5 5 curl https://mise.jdx.dev/install.sh.sig | gpg --decrypt >"$install_script"; then
      fatal "Could not download or verify mise install script"
    fi
  fi
  (
    set +e
    if ! retry 5 5 sh "$install_script"; then
      local distro
      if [[ -z $CI ]]; then
        fatal "Could not install mise"
      fi
      set -e
      distro="$(grep ^ID= /etc/os-release | cut -d= -f2-)"
      if [[ $distro != "ubuntu" ]]; then
        fatal "Could not install mise"
      fi
      warn "Installing mise via apt, mise will be installed to /usr/bin/mise instead"
      install_mise_via_apt
    fi
  )
}

# Install mise via apt for Debian-based Linux distros (including Ubuntu).
# WARNING(2025-07-21): this might need to change in the near future.
# See: https://github.com/jdx/mise/discussions/5722
install_mise_via_apt() {
  local keyrings_dir=/etc/apt/keyrings
  sudo install --directory --mode=755 "$keyrings_dir"
  wget --quiet --output-document - https://mise.jdx.dev/gpg-key.pub | gpg --dearmor | sudo tee "$keyrings_dir"/mise-archive-keyring.gpg 1>/dev/null
  echo "deb [signed-by=$keyrings_dir/mise-archive-keyring.gpg arch=$(dpkg --print-architecture)] https://mise.jdx.dev/deb stable main" | sudo tee /etc/apt/sources.list.d/mise.list
  sudo apt update
  sudo apt install --yes mise
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

  local config_file="${MISE_GLOBAL_CONFIG_FILE:-$HOME/.config/mise/config.toml}"

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
    local config_key="tools.$slug.$key"
    info_sub "Mise: setting '$config_key' to '$value' in $config_file"
    mise config set --file="$config_file" "$config_key" "$value"
  done
}

# find_mise returns the path to the mise binary if it is installed.
find_mise() {
  if command -v mise >/dev/null 2>&1; then
    command -v mise
  elif [[ -x $HOME/.local/bin/mise ]]; then
    echo "$HOME/.local/bin/mise"
  elif [[ -x /usr/local/bin/mise ]]; then
    echo "/usr/local/bin/mise"
  else
    return 1
  fi
}

# find_tool TOOL_NAME
#
# Prints the path to a tool from either PATH or in the
# mise environment.
find_tool() {
  local tool_name="$1"
  if ! command -v "$tool_name" 2>/dev/null; then
    local mise_path
    mise_path="$(find_mise)"
    if [[ -z $mise_path ]]; then
      error "mise not found (find_tool)"
      return 1
    fi
    "$mise_path" which "$tool_name"
  fi
}

mise_install_if_needed() {
  ensure_mise_installed

  local tool_name="$1"
  local installed versions

  versions="$(mise ls --local --json "$tool_name")"
  if [[ "$versions" == "[]" ]]; then
    fatal "mise: $tool_name is not declared in mise.toml"
  fi
  installed="$(echo "$versions" | gojq --raw-output '.[] | select(.installed and .active)')"

  if [[ -z $installed ]]; then
    info "mise: installing $tool_name"
    mise install --yes "$tool_name"
  else
    info "mise: $tool_name is already installed"
  fi
}
