#!/usr/bin/env bash
# Utilities for working with asdf

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
    plugin_install "$name" || echo "Warning: Failed to install language '$name', may fail to invoke things using that language"
  done <.tool-versions
}

# asdf_install installs a plugins/version required from a top-level
# .tool-versions and all subdirectories
asdf_install() {
  readarray -t tool_versions < <(find . -name .tool-versions | grep -vE "./.bootstrap" | grep -vE "./node_modules" | grep -vE "./vendor")
  if [[ -n ${tool_versions[*]} ]]; then
    for tool_version in "${tool_versions[@]}"; do
      dir="$(dirname "$tool_version")"
      pushd "$dir" >/dev/null || exit 1
      asdf_plugins_from_tool_versions
      asdf install
      asdf reshim
      popd >/dev/null || exit 1
    done
  fi
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
