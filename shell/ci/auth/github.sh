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

# shellcheck source=../../lib/shell.sh
source "$LIB_DIR/shell.sh"

# shellcheck source=../../lib/github.sh
source "$LIB_DIR/github.sh"

# shellcheck disable=SC2119
# Why: no extra args needed to pass to ghaccesstoken in this case.
bootstrap_github_token

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
