#!/usr/bin/env bash
# Setup asdf for an environment. This can be used in both Docker and Machine executors (CircleCI)
# or other CI platforms with that notion.
set -e

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
LIB_DIR="${DIR}/../../lib"

# shellcheck source=../../lib/bootstrap.sh
source "${LIB_DIR}/bootstrap.sh"
# shellcheck source=../../lib/logging.sh
source "${LIB_DIR}/logging.sh"

# Used in CircleCI, stub it in Docker. Note: Using this docker image
# outside of a platform that has BASH_ENV as a way to carry over environment
# variables between steps will not work. This only works to allow this script
# to be called to pre-load asdf into a container.
if [[ -n $BASH_ENV ]]; then
  BASH_ENV="$HOME/.bashrc"
fi

# inject_bash_env injects asdf support into the value of BASH_ENV and sources it
inject_bash_env() {
  cat >>"$BASH_ENV" <<EOF

# Source ASDF. DO NOT REMOVE THE EMPTY LINE ABOVE. This
# ensures that we never append to an existing line.
. "$HOME/.asdf/asdf.sh"
EOF

  # Setup asdf for our current terminal session
  # shellcheck disable=SC1090
  source "$BASH_ENV"
}

# init_asdf installs asdf and ensures it's usable, preloading versions
# of plugins if configured to do so.
init_asdf() {
  info "Installing asdf"
  git clone https://github.com/asdf-vm/asdf.git ~/.asdf --branch v0.8.1

  # langauage specifics
  echo -e "npm\nyarn\n" >"$HOME/.default-npm-packages"
  echo -e "bundler\n" >"$HOME/.default-gems"
  cat >"$HOME/.default-golang-pkgs" <<EOF
github.com/golang/protobuf/protoc-gen-go@v$(get_tool_version protoc-gen-go)
github.com/pseudomuto/protoc-gen-doc/cmd/protoc-gen-doc@v$(get_tool_version protoc-gen-doc)
EOF

  inject_bash_env

  # Install preloaded versions, usually used for docker executors
  # Example: PRELOAD_VERSIONS: "golang@1.17.1 ruby@2.6.6"
  if [[ -n $PRELOAD_VERSIONS ]]; then
    info "Preloading language versions"
    # IDEA(jaredallard): We could probably JIT install the plugin here?
    for preload in $PRELOAD_VERSIONS; do
      # shellcheck disable=SC2016
      language="$(awk -F '@' '{ print $1 }' <<<"$preload")"
      # shellcheck disable=SC2016
      version="$(aws -F '@' '{ print $2 }' <<<"$preload")"

      info_sub "$preload"

      # Ensure the plugin (language) exists and install the version
      plugin_install || exit 1
      asdf install "$language" "$version" || exit 1
    done
  fi
}

# plugin_install installs an asdf plugin
plugin_install() {
  name="$1"
  asdf plugin-add "$name"
}

# plugins_from_tool_versions installs all plugins from .tool-versions
plugins_from_tool_versions() {
  while read -r line; do
    name="$(awk '{ print $1 }' <<<"$line")"
    version="$(awk '{ print $2 }' <<<"$line")"
    plugin_install "$name" || warn "Failed to install language '$name', may fail to invoke things using that language"
  done <.tool-versions
}

# Install asdf if it doesn't exist.
if ! command -v asdf >/dev/null; then
  init_asdf
else
  # Ensure that steps are using asdf. init_asdf above calls this.
  inject_bash_env
fi

if [[ -e ".tool-versions" ]]; then
  info "Installing languages/plugins from .tool-versions"
  # Best effort install the plugins before doing anything else.
  plugins_from_tool_versions
  asdf install
  asdf reshim
fi
