#!/usr/bin/env bash
# Sets up vault authn
set -e

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
DEVBASE_LIB_DIR="$DIR/../../lib"

# shellcheck source=../../lib/box.sh
source "$DEVBASE_LIB_DIR/box.sh"

# shellcheck source=../../lib/shell.sh
source "$DEVBASE_LIB_DIR/shell.sh"

if [[ -n $VAULT_ROLE_ID ]] && [[ -n $VAULT_SECRET_ID ]]; then
  # Writes the token to ~/.vault-token, which later CI steps and tools read.
  # The role and secret IDs are passed in the environment, not as arguments.
  VAULT_ADDR="$(get_box_field devenv.vault.addressCI)" "$DIR/../../vault.sh" login-approle
else
  echo "Skipped: VAULT_ROLE_ID or VAULT_SECRET_ID is not set."
fi
