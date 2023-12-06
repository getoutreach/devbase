#!/usr/bin/env bash
# Utilities for working with asdf

# LIB_DIR is the directory that shell script libraries live in.
LIB_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"

# shellcheck source=./shell.sh
source "${LIB_DIR}/shell.sh"

ensure_bash_5_or_greater

# asdf_plugins_list stores a list of all asdf plugins
# this is done to speed up the plugin install
asdf_plugins_list=""

# asdf_plugins_list_regenerate regenerates the list of plugins
asdf_plugins_list_regenerate() {
  asdf_plugins_list=$(asdf plugin list 2>/dev/null || echo "")
}

# Populate the list of plugins once
if [[ -z $asdf_plugins_list ]]; then
  asdf_plugins_list_regenerate
fi

# read_all_asdf_tool_versions combines all .tool-versions found in this directory
# and the child directories minus node_modules and vendor.
# This prints the plugin, then the version, each separated by a newline.
read_all_asdf_tool_versions() {
  find . -type d \( -path ./.git -o -path ./vendor -o -path ./node_modules \) -prune -o \
    -name .tool-versions -exec cat {} \; |
    grep -Ev "^#|^$" | sort | uniq | awk '{ print $1 } { print $2 }'
}

# asdf_get_version_from_devbase returns the version of a tool from the devbase
# .tool-versions file without influencing all versions of other tools
asdf_get_version_from_devbase() {
  local tool_name="$1"

  # Support executing Go from devbase.
  if [[ $tool_name == "go" ]]; then
    tool_name="golang"
  fi

  # Why: We're OK with this being the way it is.
  # shellcheck disable=SC2155
  local devbase_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." >/dev/null 2>&1 && pwd)"

  # Check if we have a version override in the repository
  repo_version_override=$(grep -E "^$tool_name " ".tool-versions" 2>/dev/null | head -n1 | awk '{print $2}')
  if [[ -n $repo_version_override ]]; then
    echo "$repo_version_override"
    return
  fi

  # Otherwise, use the version from devbase
  grep -E "^$tool_name " "$devbase_dir/.tool-versions" | head -n1 | awk '{print $2}'
}

# asdf_devbase_exec runs asdf_devbase_run but execs the command instead
# of running it as a subprocess.
asdf_devbase_exec() {
  asdf_tool_env_var "$1"
  exec "$@"
}

# asdf_devbase_run executes a command with the versions from the devbase
# .tool-versions file. This will fail if the tool isn't installed, so callers
# should invoke asdf_devbase_ensure first.
asdf_devbase_run() {
  asdf_tool_env_var "$1"
  "$@"
}

# asdf_tool_env_var exports an environment variable to have the provided
# tool version be used in asdf. This mutates the current shell's
# environment variables by exporting the variable.
#
# See: https://asdf-vm.com/manage/versions.html#set-current-version
asdf_tool_env_var() {
  local tool="$1"
  # Why: We're OK with this being the way it is.
  # shellcheck disable=SC2155
  local tool_env_var="$(tr '[:lower:]-' '[:upper:]_' <<<"$tool")"

  # Why: We're OK with this being the way it is.
  # shellcheck disable=SC2155
  local version="$(asdf_get_version_from_devbase "$tool")"
  if [[ -z $version ]]; then
    echo "No version found for $tool in devbase .tool-versions file"
    return 1
  fi

  export "ASDF_${tool_env_var}_VERSION"="${version}"
}

# asdf_devbase_ensure ensures that all versions of tools are installed from
# .tool-version files in the current directory and all subdirectories.
asdf_devbase_ensure() {
  readarray -t asdf_entries < <(read_all_asdf_tool_versions)
  local need_reshim=0
  for ((i = 0; i < "${#asdf_entries[@]}"; i = (i + 2))); do
    # Why: We're OK not declaring separately here.
    # shellcheck disable=SC2155
    local plugin="${asdf_entries[$i]}"
    # Why: We're OK not declaring separately here.
    # shellcheck disable=SC2155
    local version="${asdf_entries[i + 1]}"

    if [[ -z $plugin ]]; then
      echo "No plugin found in devbase .tool-versions file"
      exit 1
    fi
    if [[ -z $version ]]; then
      echo "No version found for $plugin in devbase .tool-versions file"
      exit 1
    fi

    # Install the plugin first, so we can install the version
    # Note: This only runs if the plugin doesn't already exist
    asdf_plugin_install "$plugin" || echo "Warning: Failed to install language '$name', may fail to invoke things using that language"

    # Install the version if it doesn't already exist
    if ! asdf list "$plugin" 2>/dev/null | grep -qE "$version$"; then
      need_reshim=1

      echo "ensure_asdf: Installing $plugin $version"
      # Install the language, retrying w/ AMD64 emulation if on macOS or just retrying on failure once.
      asdf install "$plugin" "$version" || asdf_install_retry "$plugin" "$version"
    fi
  done

  if [ "$need_reshim" == 1 ]; then
    # Reshim to ensure that the correct versions are used
    asdf reshim
  fi
}

# asdf_install installs a plugins/version required from a top-level
# .tool-versions and all subdirectories.
# Deprecated: Use asdf_devbase_ensure instead.
asdf_install() {
  asdf_devbase_ensure
}

# asdf_install_retry attempts to retry on certain platforms
asdf_install_retry() {
  local plugin="$1"
  local version="$2"

  # Failed to install, try again once in case of network flakiness
  if ! asdf install "$plugin" "$version"; then
    echo
    echo "Failed to install $plugin $version"

    # If we're on macOS and we're on an M1, note that the user should try
    # installing with AMD64 emulation. We don't do this ourself because it's
    # slow to run commands under emulation and we can't tell if that is the reason.
    if [[ "$(uname -s)" == "Darwin" ]] && [[ "$(uname -m)" == "arm64" ]]; then
      echo "$(tput bold)Note:$(tput sgr0) This may be due to the plugin not supporting your current architecture."
      echo "      This is likely the case if you're seeing \"not found\" or 404 errors and"
      echo "      you're sure that the version you're trying to install exists. Using the"
      echo "      plugin with AMD64 emulation may help, however this will result in all"
      echo "      commands being run under emulation which is slow."
      echo
      echo "      You can try installing the plugin with the following command:"
      echo
      echo "      arch -x86_64 asdf install $plugin $version"
    fi
  fi
}

# asdf_plugin_install installs an asdf plugin
asdf_plugin_install() {
  name="$1"

  # NOOP if it already exists
  if grep -qE "^$name$" <<<"$asdf_plugins_list"; then
    return
  fi

  asdf plugin-add "$name"
  asdf_plugins_list_regenerate
}
