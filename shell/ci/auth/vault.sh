#!/usr/bin/env bash
# Sets up vault authn
set -e

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
DEVBASE_LIB_DIR="$DIR/../../lib"

# shellcheck source=../../lib/box.sh
source "$DEVBASE_LIB_DIR/box.sh"

# shellcheck source=../../lib/mise.sh
source "$DEVBASE_LIB_DIR/mise.sh"

# shellcheck source=../../lib/shell.sh
source "$DEVBASE_LIB_DIR/shell.sh"

if [[ -n $VAULT_ROLE_ID ]] && [[ -n $VAULT_SECRET_ID ]]; then
  VAULT_ADDR="$(get_box_field devenv.vault.addressCI)" "$(find_tool vault)" write auth/approle/login \
    role_id="$VAULT_ROLE_ID" secret_id="$VAULT_SECRET_ID" -format=json |
    "$(find_tool gojq)" --raw-output .auth.client_token >"$HOME/.vault-token"
else
  echo "Skipped: VAULT_ROLE_ID or VAULT_SECRET_ID is not set."
fi
