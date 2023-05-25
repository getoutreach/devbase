#!/usr/bin/env bash
# Utilities for working with asdf

VERSION_MANAGER="asdf"
if command -v rtx @ >/dev/null && [[ -z $DEVBASE_DISABLE_RTX ]]; then
  if [[ -z $DEVBASE_WARNED_RTX ]]; then
    export DEVBASE_WARNED_RTX=1
    echo "[devbase] rtx found in path, using rtx instead of asdf. This is a beta feature." >&2
  fi
  VERSION_MANAGER="rtx"

  # alias asdf to rtx because it is compatible with asdf, for the most part.
  # Note: When this code is rewritten in Go, we should have actual interfaces
  # for the version managers and not have to alias.
  asdf() {
    rtx "$@"
  }
fi

# asdf_plugins_list stores a list of all asdf plugins
# this is done to speed up the plugin install
asdf_plugins_list=""

# asdf_plugins_list_regenerate regenerates the list of plugins
asdf_plugins_list_regenerate() {
  asdf_plugins_list=$(asdf plugin list 2>/dev/null || echo "")
}

# Populate the list of plugins once
asdf_plugins_list_regenerate

# read_all_asdf_tool_versions combines all .tool-versions found in this directory
# and the child directories minus node_modules and vendor.
# Then strip the comments and run uniq.
read_all_asdf_tool_versions() {
  find . -type d \( -path ./.git -o -path ./vendor -o -path ./node_modules \) -prune -o \
    -name .tool-versions -exec cat {} \; |
    grep -Ev "^#|^$" | sort | uniq
}

# asdf_get_version_from_devbase returns the version of a tool from the devbase
# .tool-versions file without influencing all versions of other tools
asdf_get_version_from_devbase() {
  local tool_name="$1"
  # Why: We're OK with this being the way it is.
  # shellcheck disable=SC2155
  local devbase_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." >/dev/null 2>&1 && pwd)"
  grep -E "^$tool_name " "$devbase_dir/.tool-versions" | awk '{print $2}'
}

# asdf_devbase_exec executes a command with the versions from the devbase
# .tool-versions file without influencing all versions of other tools
asdf_devbase_exec() {
  local tool="$1"
  # Why: We're OK with this being the way it is.
  # shellcheck disable=SC2155
  local tool_env_var="$(echo "$tool" | tr '[:lower:]-' '[:upper:]_')"

  # Why: We're OK with this being the way it is.
  # shellcheck disable=SC2155
  local version="$(asdf_get_version_from_devbase "$tool")"
  if [[ -z $version ]]; then
    echo "No version found for $tool in devbase .tool-versions file"
    exit 1
  fi

  if [[ $VERSION_MANAGER == "asdf" ]]; then
    export "ASDF_${tool_env_var}_VERSION"="${version}"

    # Ensure that the tool and/or plugin is installed
    asdf_devbase_ensure

    exec "$@"
  else
    exec rtx exec "$tool@$version" -- "$@"
  fi
}

# asdf_devbase_ensure ensures that the versions from the devbase
# .tool-versions file are installed
asdf_devbase_ensure() {
  readarray -t asdf_entries < <(read_all_asdf_tool_versions)
  for entry in "${asdf_entries[@]}"; do
    # Why: We're OK not declaring separately here.
    # shellcheck disable=SC2155
    local plugin="$(awk '{ print $1 }' <<<"$entry")"
    # Why: We're OK not declaring separately here.
    # shellcheck disable=SC2155
    local version="$(awk '{ print $2 }' <<<"$entry")"

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
    if ! asdf list "$plugin" | grep -qE "$version"; then
      # Install the language, retrying w/ AMD64 emulation if on macOS or just retrying on failure once.
      asdf install "$plugin" "$version" || asdf_install_retry "$plugin" "$version"
    fi
  done
}

# asdf_install installs a plugins/version required from a top-level
# .tool-versions and all subdirectories
asdf_install() {
  readarray -t asdf_entries < <(read_all_asdf_tool_versions)
  for entry in "${asdf_entries[@]}"; do
    # Why: We're OK not declaring separately here.
    # shellcheck disable=SC2155
    local plugin="$(awk '{ print $1 }' <<<"$entry")"
    # Why: We're OK not declaring separately here.
    # shellcheck disable=SC2155
    local version="$(awk '{ print $2 }' <<<"$entry")"

    if [[ -z $plugin ]]; then
      echo "No plugin found in devbase .tool-versions file"
      exit 1
    fi
    if [[ -z $version ]]; then
      echo "No version found for $plugin in devbase .tool-versions file"
      exit 1
    fi

    # Install the plugin first, so we can install the version
    asdf_plugin_install "$plugin" || echo "Warning: Failed to install language '$name', may fail to invoke things using that language"

    # Install the language, retrying w/ AMD64 emulation if on macOS or just retrying on failure once.
    asdf install "$plugin" "$version" || asdf_install_retry "$plugin" "$version"
  done

  echo "Reshimming asdf (this may take awhile ...)"
  asdf reshim
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
      echo "      arch -x86_64 $VERSION_MANAGER install $plugin $version"
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

  asdf plugin add "$name"
  asdf_plugins_list_regenerate
}
