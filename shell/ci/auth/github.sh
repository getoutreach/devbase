#!/usr/bin/env bash
set -e

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
LIB_DIR="${DIR}/../../lib"

# shellcheck source=../../lib/bootstrap.sh
source "${LIB_DIR}/bootstrap.sh"

# shellcheck source=../../lib/logging.sh
source "$LIB_DIR/logging.sh"

# shellcheck source=../../lib/mise.sh
source "$LIB_DIR/mise.sh"

# Fetch the token from ghaccesstoken if not set.
if [[ -z $GITHUB_TOKEN ]]; then
  ghaccesstoken_version="$(get_tool_version getoutreach/ci)"
  mise_tool_config_set ubi:getoutreach/ci version "$ghaccesstoken_version" exe ghaccesstoken
  install_tool_with_mise ubi:getoutreach/ci "$ghaccesstoken_version"
  GITHUB_TOKEN="$("$(mise which ghaccesstoken)" --skip-update token)"
fi

# Configure the gh CLI, and tools that depend on it
mkdir -p "$HOME/.config/gh"
cat >"$HOME/.config/gh/hosts.yml" <<EOF
github.com:
  user: devbase
  oauth_token: $GITHUB_TOKEN
EOF

# Configure the legacy path
mkdir -p "$HOME/.outreach"
echo "$GITHUB_TOKEN" >"$HOME/.outreach/github.token"
