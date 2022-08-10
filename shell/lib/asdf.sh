#!/usr/bin/env bash
# Utilities for working with asdf

# Deprecated: Use asdf_install instead.
# asdf_plugins_from_tool_versions installs all plugins from .tool-versions
asdf_plugins_from_tool_versions() {
  while read -r line; do
    # Skip comments
    if grep -E "^#" <<<"$line" >/dev/null; then
      continue
    fi

    # Why: We're OK not declaring separately here.
    # shellcheck disable=SC2155
    local name="$(awk '{ print $1 }' <<<"$line")"
    asdf_plugin_install "$name" || echo "Warning: Failed to install language '$name', may fail to invoke things using that language"
  done <.tool-versions
}

# asdf_install installs a plugins/version required from a top-level
# .tool-versions and all subdirectories
asdf_install() {
  # Combine all .tool-versions found in this directory and the child directories
  # minus node_modules and vendor. Then strip the comments and run uniq.
  readarray -t asdf_entries < <(find . -name .tool-versions | grep -vE "./node_modules" | grep -vE "./vendor" | xargs cat | grep -Ev "^#" | sort | uniq)
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
  if asdf plugin list | grep -E "^$name$" >/dev/null; then
    return
  fi

  asdf plugin-add "$name"
}
