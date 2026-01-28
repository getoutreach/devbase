#!/usr/bin/env bash
# Install dependencies that are required in a machine environment.
# These are usually already installed in a CircleCI docker image.
set -e

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
LIB_DIR="$DIR/../lib"
ROOT_DIR="$DIR/../.."

# shellcheck source=../lib/bootstrap.sh
source "$LIB_DIR"/bootstrap.sh

# shellcheck source=../lib/github.sh
source "$LIB_DIR"/github.sh

# shellcheck source=../lib/logging.sh
source "$LIB_DIR"/logging.sh

# shellcheck source=../lib/mise.sh
source "$LIB_DIR"/mise.sh

# shellcheck source=../lib/shell.sh
source "$LIB_DIR"/shell.sh

if [[ $OSTYPE == "darwin"* ]]; then
  brew install bash docker gnupg gnu-sed
  # Rosetta is required for awscli installed by mise
  softwareupdate --install-rosetta --agree-to-license
fi

ensure_mise_installed
devbase_configure_global_tools

if [[ ($OSTYPE == "darwin"* && -z ${ALLOW_MISE_TO_MANAGE_TOOL_VERSIONS:-}) || -n $DEVBASE_PRELOAD_GO_FOR_MISE ]]; then
  install_tool_with_mise go "$(grep ^golang "$ROOT_DIR/.tool-versions" | awk '{print $2}')"
fi
run_mise trust --env devbase --cd "$ROOT_DIR"
run_mise install --cd "$HOME" github-cli github:getoutreach/ci gojq

bootstrap_github_token

info "Installing tools via mise required in machine environment"
run_mise install

# Remove the existing yq, if it already exists
# (usually the Go Version we don't support)
info "Removing existing Go-based (incompatible) yq"
sudo rm -f "$(command -v yq)"

info "Installing yq (Python)"
install_tool_with_mise uv
mise config set settings.pipx.uvx true
install_tool_with_mise pipx:yq

if ! command -v vault >/dev/null 2>&1; then
  install_tool_with_mise vault
  sudo rm -rf /opt/vault
fi

# install AWS CLI

if ! command -v aws >/dev/null; then
  install_tool_with_mise aws-cli
fi

# Tiny app to work around GitHub token rate limits
install_tool_with_mise wait-for-gh-rate-limit
