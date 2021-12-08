#!/usr/bin/env bash
set -e

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
SHELL_DIR="$DIR/../.."
LIB_DIR="${DIR}/../../lib"

# shellcheck source=../../lib/bootstrap.sh
source "${LIB_DIR}/bootstrap.sh"

# Fetch the token from ghaccesstoken if not set.
if [[ -z $GITHUB_TOKEN ]]; then
  GITHUB_TOKEN=$("$SHELL_DIR/gobin.sh" "github.com/getoutreach/ci/cmd/ghaccesstoken@$(get_tool_version "getoutreach/ci")" token)
fi

mkdir -p "$HOME/.outreach"
echo "$GITHUB_TOKEN" >"$HOME/.outreach/github.token"
