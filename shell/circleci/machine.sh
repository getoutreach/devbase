#!/usr/bin/env bash
# Install dependencies that are required in a machine environment.
# These are usually already installed in a CircleCI docker image.
set -e

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
LIB_DIR="$DIR/../lib"
ROOT_DIR="$DIR/../.."

# shellcheck source=../lib/logging.sh
source "$LIB_DIR"/logging.sh

# shellcheck source=../lib/mise.sh
source "$LIB_DIR"/mise.sh

if [[ "$OSTYPE" == "darwin"* ]]; then
  brew install bash gnupg
fi

install_tool_with_mise github-cli "$(grep ^gh: "$ROOT_DIR"/versions.yaml | awk '{print $2}')"

info "Installing yq (Python)"
# Remove the existing yq, if it already exists
# (usually the Go Version we don't support)
info_sub "Removing existing Go-based (incompatible) yq"
sudo rm -f "$(command -v yq)"

install_tool_with_mise uv
mise config set settings.pipx.uvx true
install_tool_with_mise pipx:yq

# Install gojq as that's preferred over yq
if ! command -v gojq >/dev/null 2>&1; then
  install_tool_with_mise gojq "$(grep ^gojq: "$ROOT_DIR"/versions.yaml | awk '{print $2}')"
fi

if ! command -v vault >/dev/null 2>&1; then
  install_tool_with_mise vault
  sudo rm -rf /opt/vault
fi

# install AWS CLI

if ! command -v aws >/dev/null; then
  install_tool_with_mise aws-cli
fi
