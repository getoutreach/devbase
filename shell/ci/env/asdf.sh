#!/usr/bin/env bash
# Setup asdf for an environment. This can be used in both Docker and Machine executors (CircleCI)
# or other CI platforms with that notion.
set -e

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
LIB_DIR="${DIR}/../../lib"

# shellcheck source=../../lib/logging.sh
source "${LIB_DIR}/logging.sh"

defaultPlugins=("golang" "ruby" "nodejs")

echo "Setting up ASDF"
git clone https://github.com/asdf-vm/asdf.git ~/.asdf --branch v0.8.1

cat >>"$BASH_ENV" <<EOF

# Source ASDF. DO NOT REMOVE THE EMPTY LINE ABOVE. This
# ensures that we never append to an existing line.
. "$HOME/.asdf/asdf.sh"
EOF

# langauage specifics
echo -e "npm\nyarn\n" >"$HOME/.default-npm-packages"
echo -e "bundler\n" >"$HOME/.default-gems"

# Setup asdf for our current terminal session
# shellcheck disable=SC1090
source "$BASH_ENV"

info "Setting up ASDF plugins"
for plugin in "${defaultPlugins[@]}"; do
  info_sub "$plugin"
  asdf plugin-add "$plugin"
done

# Install preloaded versions, usually used for docker executors
# Example: PRELOAD_VERSIONS: "golang@1.17.1 ruby@2.6.6"
if [[ -z $PRELOAD_VERSIONS ]]; then
  info "Preloading language versions"
  # IDEA(jaredallard): We could probably JIT install the plugin here?
  for preload in $PRELOAD_VERSIONS; do
    # shellcheck disable=SC2016
    language="$(awk -F '@' '{ print $1 }' <<<"$preload")"
    # shellcheck disable=SC2016
    version="$(aws -F '@' '{ print $2 }' <<<"$preload")"
    asdf install "$language" "$version"
  done
fi

if [[ -e ".tool-versions" ]]; then
  info "Setting up required versions"
  # IDEA(jaredallard): We could probably JIT install the plugin here?
  asdf install
  asdf reshim
fi
