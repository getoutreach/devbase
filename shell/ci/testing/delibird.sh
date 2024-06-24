#!/usr/bin/env bash
# Runs the delibird log/tracing/session-recording uploader if
# configured to do so in the box configuration.
#
# Automatically handles installation of the delibird log uploader if it
# is not already installed.
set -euo pipefail

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
LIB_DIR="$DIR/../../lib"

# shellcheck source=../../lib/bootstrap.sh
source "$LIB_DIR/bootstrap.sh"
# shellcheck source=../../lib/logging.sh
source "$LIB_DIR/logging.sh"
# shellcheck source=../../lib/github.sh
source "$LIB_DIR/github.sh"
# shellcheck source=../../lib/box.sh
source "$LIB_DIR/box.sh"

# DELIBIRD_ENABLED denotes if the delibird log uploader should be
# enabled or not. If the value is "true", then the delibird log uploader
# will be enabled. If the value is "false", then the delibird log
# uploader will be disabled.
DELIBIRD_ENABLED=$(get_box_field "delibird.enabled")

# VAULT_ADDR is the address of the Vault instance to use for fetching
# the delibird token during the installation of the delibird log
# uploader.
VAULT_ADDR=${VAULT_ADDR:-$(get_box_field devenv.vault.address)}
export VAULT_ADDR

# install_delibird installs the delibird log uploader.
install_delibird() {
  install_latest_github_release getoutreach/orc false delibird

  # tokenPath is the path that the delibird token should be written to.
  local tokenPath="$HOME/.outreach/.delibird/token"
  mkdir -p "$(dirname "$tokenPath")"

  # Fetch the delibird token from Vault.
  DELIBIRD_TOKEN=$(vault kv get -format=json deploy/delibird/development/upload | jq -r '.data.data.token')
  if [[ -z $DELIBIRD_TOKEN ]]; then
    echo "Error: Failed to fetch delibird token from Vault." \
      "Please ensure that the deploy/delibird/development/upload secret exists and" \
      "that the shell/ci/auth/vault.sh script has been ran to configure Vault access." >&2
    exit 1
  fi

  echo -n "$DELIBIRD_TOKEN" >"$tokenPath"
}

# Exit if we're not enabled.
if [[ $DELIBIRD_ENABLED != "true" ]]; then
  exit 0
fi

# Otherwise, check if the uploader is installed. If it isn't, attempt to
# install it.
if ! command -v delibird &>/dev/null; then
  install_delibird
fi

info "Running the delibird log uploader"

# Ensure the logs directory exists.
mkdir -p "$HOME/.outreach/logs"

exec delibird --run-once start
