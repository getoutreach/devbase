#!/usr/bin/env bash
# Utilities for working with asdf

# asdf_plugins_list stores a list of all asdf plugins
# this is done to speed up the plugin install
asdf_plugins_list=$(asdf plugin list 2>/dev/null || echo "")

# read_all_asdf_tool_versions combines all .tool-versions found in this directory
# and the child directories minus node_modules and vendor.
# Then strip the comments and run uniq.
read_all_asdf_tool_versions() {
  find . -name .tool-versions | grep -vE "./node_modules" | grep -vE "./vendor" | xargs cat | grep -Ev "^#" | sort | uniq
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

  if [[ "$(uname -s)" == "Darwin" ]] && [[ "$(uname -m)" == "arm64" ]]; then
    arch -x86_64 asdf install "$plugin" "$version"
    return $?
  fi

  # Not a supported retry, so just try again once and pray.
  asdf install "$plugin" "$version"
}

# asdf_plugin_install installs an asdf plugin
asdf_plugin_install() {
  name="$1"

  # NOOP if it already exists
  if grep -qE "^$name$" <<<"$asdf_plugins_list"; then
    return
  fi

  asdf plugin-add "$name"
}
