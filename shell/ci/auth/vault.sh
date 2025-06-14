#!/usr/bin/env bash
# Sets up vault authn
set -e

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
DEVBASE_LIB_DIR="$DIR/../../lib"

# shellcheck source=../../lib/box.sh
source "$DEVBASE_LIB_DIR/box.sh"

if [[ -n $VAULT_ROLE_ID ]] && [[ -n $VAULT_SECRET_ID ]]; then
  VAULT_ADDR="$(get_box_field devenv.vault.addressCI)" vault write auth/approle/login \
    role_id="$VAULT_ROLE_ID" secret_id="$VAULT_SECRET_ID" -format=json |
    jq .auth.client_token -r >"$HOME/.vault-token"
else
  echo "Skipped: VAULT_ROLE_ID or VAULT_SECRET_ID is not set."
fi
