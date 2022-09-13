#!/usr/bin/env bash
# This script ensures that all of the entries in .tool-versions have
# been installed, and if not, that they are installed.
# This is meant to be ran before calling out to mage or other tools.
set -e

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"

# shellcheck source=../shell/lib/asdf.sh
source "$DIR/../shell/lib/asdf.sh"

ensure() {
  readarray -t asdf_entries < <(read_all_asdf_tool_versions)
  for entry in "${asdf_entries[@]}"; do
    # Why: We're OK not declaring separately here.
    # shellcheck disable=SC2155
    local plugin="$(awk '{ print $1 }' <<<"$entry")"
    # Why: We're OK not declaring separately here.
    # shellcheck disable=SC2155
    local version="$(awk '{ print $2 }' <<<"$entry")"

    # Install the plugin first, so we can install the version
    # Note: This only runs if the plugin doesn't already exist
    asdf_plugin_install "$plugin" || echo "Warning: Failed to install language '$name', may fail to invoke things using that language"

    # If the version doesn't exist, install it.
    # Note: we don't use asdf list <plugin> here because checking the file system
    # entry is ~90% faster than running the asdf command.
    if [[ ! -d "$ASDF_DIR/installs/$plugin/$version" ]]; then
      # Install the language, retrying w/ AMD64 emulation if on macOS or just retrying on failure once.
      asdf install "$plugin" "$version" || asdf_install_retry "$plugin" "$version"
    fi
  done
}

ensure
