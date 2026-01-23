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

    local mise_bin="${MISE_INSTALL_PATH:-$HOME/.local/bin/mise}"

    info "Installing mise to $mise_bin"

    install_mise

    unset MISE_INSTALL_PATH

    "$mise_bin" --version

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
      error "Could not import mise GPG release key"
      install_mise_via_apt_if_ubuntu_in_ci
    fi
    # ensure the install script is signed with the mise release key
    if ! download_mise_install_script | gpg --decrypt >"$install_script"; then
      error "Could not download or verify mise install script"
      install_mise_via_apt_if_ubuntu_in_ci
    fi
  fi
  (
    set +e
    if ! retry 5 5 sh "$install_script"; then
      install_mise_via_apt_if_ubuntu_in_ci
    fi
  )
  run_mise settings set http_retries 3
  run_mise settings set use_versions_host_track false
}

# Fetch a URL via either curl or wget, with retries, with the response body going to stdout.
http_fetch() {
  local url="$1"
  if command_exists curl; then
    retry 5 5 curl --fail --silent --show-error --location "$url"
  elif command_exists wget; then
    # Using short flags because of busybox compatibility.
    # -q: --quiet; -O -: --output-document - (stdout)
    retry 5 5 wget -q -O - "$url"
  else
    error "Could not fetch URL '$url', missing either curl or wget"
    return 1
  fi
}

# Download mise script with retries, assuming either curl or wget is installed.
download_mise_install_script() {
  http_fetch "https://mise.jdx.dev/install.sh.sig"
}

# Install mise via apt if running in CI on Ubuntu.
install_mise_via_apt_if_ubuntu_in_ci() {
  local distro
  if ! in_ci_environment; then
    warn "Falling back to apt installation of mise is only supported in CI environments"
    return 1
  fi
  set -e
  distro="$(grep ^ID= /etc/os-release | cut -d= -f2-)"
  if [[ $distro != "ubuntu" ]]; then
    warn "Falling back to apt installation of mise is only supported on Ubuntu"
    return 1
  fi
  warn "Installing mise via apt, mise will be installed to /usr/bin/mise instead"
  install_mise_via_apt
}

# Install mise via apt for Debian-based Linux distros (including Ubuntu).
install_mise_via_apt() {
  local keyrings_dir=/etc/apt/keyrings
  sudo install --directory --mode=755 "$keyrings_dir"
  http_fetch https://mise.jdx.dev/gpg-key.pub | gpg --dearmor | sudo tee "$keyrings_dir"/mise-archive-keyring.gpg 1>/dev/null
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

  if ! run_mise use --global "$tool"; then
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
  if command_exists mise; then
    command -v mise
  elif [[ -x $HOME/.local/bin/mise ]]; then
    echo "$HOME/.local/bin/mise"
  elif [[ -x /usr/local/bin/mise ]]; then
    echo "/usr/local/bin/mise"
  else
    return 1
  fi
}

# run_mise ARGS...
#
# Runs `mise`. If in CI, `MISE_GITHUB_TOKEN` or `GITHUB_TOKEN` is set, and
# `wait-for-gh-rate-limit` is installed, makes sure that the token
# isn't rate limited before calling `mise`.
run_mise() {
  local mise_path
  mise_path="$(find_mise)"
  if in_ci_environment && [[ -n ${MISE_GITHUB_TOKEN:-} || -n ${GITHUB_TOKEN:-} ]]; then
    local wait_for_gh_rate_limit
    set +e
    wait_for_gh_rate_limit="$(find_tool wait-for-gh-rate-limit)"
    set -e
    if [[ -n $wait_for_gh_rate_limit ]]; then
      # Send output to stderr so that it doesn't affect stdout of mise
      "$wait_for_gh_rate_limit" >&2
    fi
  fi
  "$mise_path" "$@"
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
    "$mise_path" which "$tool_name" 2>/dev/null
  fi
}

# mise_exec_tool(toolName[, args...])
#
# Runs `mise exec` on a tool defined in `mise.devbase.toml` (equivalent
# to MISE_ENV=devbase). Assumes the binary and the tool name are the same,
# and that `github.sh` is sourced.
mise_exec_tool() {
  local toolName="$1"
  shift
  mise_exec_tool_with_bin "$toolName" "$toolName" "$@"
}

# mise_exec_tool_with_bin(toolName, binName[, args...])
#
# Runs `mise exec` on a tool defined in `mise.devbase.toml` (equivalent
# to MISE_ENV=devbase). Assumes `github.sh` is sourced.
mise_exec_tool_with_bin() {
  local toolName="$1"
  shift
  local binName="$1"
  shift

  local binPath
  set +e
  binPath="$(find_tool "$binName")"
  set -e

  if [[ $binPath == "$(asdf_shim_dir)"* ]]; then
    remove_asdf_shim_from_ci "$binName"
    binPath="$(find_tool "$binName")"
  fi

  if [[ -n $binPath ]]; then
    "$binPath" "$@"
  else
    MISE_GITHUB_TOKEN=$(github_token) run_mise exec "$toolName@$(devbase_tool_version_from_mise "$toolName")" -- "$binName" "$@"
  fi
}

asdf_shim_dir() {
  echo "${ASDF_DIR:-$HOME/.asdf}/shims"
}

asdf_shim_path() {
  local binName="$1"
  echo "$(asdf_shim_dir)/$binName"
}

# Removes the given shim from CI if it exists. This is because `asdf`
# shims take precedence in the PATH.
remove_asdf_shim_from_ci() {
  local asdfShim binName="$1"
  asdfShim="$(asdf_shim_path "$binName")"
  if in_ci_environment && [[ -f $asdfShim ]]; then
    rm "$asdfShim"
  fi
}

# Runs mise in the context of the repo's devbase directory and devbase env
devbase_mise() {
  local subcommand="$1"
  shift

  if [[ -z $subcommand ]]; then
    fatal "Running devbase_mise requires at least one argument"
  fi

  run_mise "$subcommand" --cd "$(get_devbase_directory)" --env devbase "$@"
}

# Determines the requested version of a tool as defined in
# devbase's `mise.devbase.toml`.
devbase_tool_version_from_mise() {
  local toolName="$1"
  devbase_mise ls --local --json |
    gojq --raw-output ".[\"$toolName\"][] | "'select(.source.path | endswith("mise.devbase.toml")).requested_version'
}

# Installs a given tool via `mise install`, assuming that it's defined
# in the local `mise.toml` file and not already installed.
mise_install_if_needed() {
  ensure_mise_installed

  local tool_name="$1"
  local installed versions

  versions="$(run_mise ls --local --json "$tool_name")"
  if [[ $versions == "[]" ]]; then
    fatal "mise: $tool_name is not declared in mise.toml"
  fi
  installed="$(echo "$versions" | gojq --raw-output '.[] | select(.installed and .active)')"

  if [[ -z $installed ]]; then
    info "mise: installing $tool_name"
    run_mise install --yes "$tool_name"
  fi
}
