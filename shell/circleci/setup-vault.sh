#!/usr/bin/env bash
# Authenticates via an approle to our development vault instance
echo "🔒 Setting up vault access"
VAULT_ADDR=https://vault-dev.outreach.cloud vault write auth/approle/login role_id="$VAULT_ROLE_ID" secret_id="$VAULT_SECRET_ID" -format=json | jq .auth.client_token -r >~/.vault-token
