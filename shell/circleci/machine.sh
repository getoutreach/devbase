#!/usr/bin/env bash
# Install dependencies that are required in a machine environment.
# These are usually already installed in a CircleCI docker image.
set -e

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
LIB_DIR="$DIR/../lib"
ROOT_DIR="$DIR/../.."

# shellcheck source=../lib/bootstrap.sh
source "$LIB_DIR"/bootstrap.sh

# shellcheck source=../lib/circleci.sh
source "$LIB_DIR"/circleci.sh

# shellcheck source=../lib/github.sh
source "$LIB_DIR"/github.sh

# shellcheck source=../lib/logging.sh
source "$LIB_DIR"/logging.sh

# shellcheck source=../lib/mise.sh
source "$LIB_DIR"/mise.sh

# shellcheck source=../lib/shell.sh
source "$LIB_DIR"/shell.sh

# shellcheck source=../lib/version.sh
source "$LIB_DIR"/version.sh

if [[ $OSTYPE == "darwin"* ]]; then
  brew install bash docker gnupg gnu-sed
  # Rosetta is required for awscli installed by mise
  softwareupdate --install-rosetta --agree-to-license
fi

ensure_mise_installed
if circleci_e2e_mode_enabled; then
  miseEnv=e2e
else
  miseEnv=devbase
fi
mise_configure_global_tools_for_env "$miseEnv"
mise_for_env "$miseEnv" trust

if [[ -z $GITHUB_TOKEN ]]; then
  # Minimum amount of tools to install to bootstrap the GitHub token
  mise_for_env "$miseEnv" install --yes github-cli github:getoutreach/ci gojq

  bootstrap_github_token

  if [[ -z $GITHUB_TOKEN ]]; then
    fatal "GitHub token not configured in environment, needed for installing tools via mise."
  fi

  if ([[ $OSTYPE == "darwin"* ]] && ! mise_manages_tool_versions) || ! command_exists go; then
    install_tool_with_mise go "$(grep ^golang "$ROOT_DIR/.tool-versions" | awk '{print $2}')"
    install_tool_with_mise node "$(grep ^nodejs "$ROOT_DIR/.tool-versions" | awk '{print $2}')"
  fi
fi

if circleci_e2e_mode_enabled; then
  info "E2E mode: skipping broad mise install; installing only tools pinned in mise.e2e.toml"
  mise_install_tools_for_env e2e
else
  info "Installing tools via mise required in machine environment"
  run_mise install --cd "$HOME"

  # Remove the existing yq, if it already exists
  # (usually the Go Version we don't support)
  info "Removing existing Go-based (incompatible) yq"
  sudo rm -f "$(command -v yq)"

  info "Installing yq (Python)"
  install_tool_with_mise uv
  mise config set settings.pipx.uvx true
  install_tool_with_mise pipx:yq
fi

if [[ -e /opt/vault ]]; then
  sudo rm -rf /opt/vault
fi
