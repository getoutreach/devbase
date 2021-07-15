#!/usr/bin/env bash
# This script should be run to provide a local configuration file if you intend to
# build/run/debug your service directly, outside of the kubernetes dev-environment.

set -e
DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
# shellcheck source=./lib/bootstrap.sh
source "$DIR/lib/bootstrap.sh"

APPNAME="$(get_app_name)"

overridePath="$(get_repo_directory)/scripts/devconfig.override.sh"
configDir="$HOME/.outreach/$APPNAME"
volumeDir="${TMPDIR:-/tmp}/$APPNAME"

# shellcheck source=./lib/logging.sh
source "$DIR/lib/logging.sh"

mkdir -p "$configDir"

VAULT_ADDR=https://vault.outreach.cloud
if [[ -n $CI ]]; then
  VAULT_ADDR=https://vault-dev.outreach.cloud
fi
export VAULT_ADDR

ensure_logged_into_vault() {

  # We redirect to stderr here to prevent mangling the output when used
  {
    # Attempt to log into vault if we aren't already.
    if ! vault kv list dev >/dev/null 2>&1; then
      local vaultVersion
      local vaultMajorVersion
      local vaultMinorVersion
      vaultVersion="$(vault version | awk '{ print $2 }' | sed -e 's:^v::g')"
      vaultMajorVersion="$(cut -d. -f1 <<<"$vaultVersion")"
      vaultMinorVersion="$(cut -d. -f2 <<<"$vaultVersion")"
      if [[ $vaultMajorVersion -lt 1 || ($vaultMajorVersion -eq 1 && $vaultMinorVersion -lt 1) ]]; then
        fatal "Please upgrade the Vault CLI. Try running 'outreach k8s install_deps'"
      fi
      info "Logging user into vault"
      vault login -method=oidc
    fi
  } 1>&2
}

get_vault_secrets() {
  # Should be path/to/key, this is fed into vault
  local key="$1"

  # Path to store the secrets at
  local path="$2"
  path="$path/$(basename "$key")"

  mkdir -p "$path"

  # shellcheck disable=SC2155
  local data="$(vault kv get -format=json "$key" | jq -cr '.data.data')"
  if [[ -z $data ]]; then
    fatal "Failed to get vault key '$key'"
  fi

  mapfile -t subKeys < <(jq -r 'keys[]' <<<"$data")
  for subKey in "${subKeys[@]}"; do
    jq -cr ".[\"$subKey\"]" <<<"$data" >"$path/$subKey"
    printf %s "$(<"$path/$subKey")" >"$path/$subKey" # this is a hack to remove trailing new lines
  done

  return 0
}

if [[ -z $CI ]]; then
  ensure_logged_into_vault
fi

info "Generating local config/secrets in '$configDir'"
envsubst="$("$DIR/gobin.sh" -p github.com/a8m/envsubst/cmd/envsubst@v1.2.0)"

info "Fetching Configuration File(s)"
DEPLOY_TO_DEV_ENVIRONMENT=local_development "$DIR/deploy-to-dev.sh" show | yq -r 'select(.kind == "ConfigMap") | .data | to_entries[] | [.key, .value] | @tsv' | "$envsubst" |
  while IFS=$'\t' read -r configFile configData; do

    saveFile="$configDir/$configFile"
    tmpFile="$saveFile.tmp"
    mergedFile="$saveFile.merged"

    echo -e "$configData" | sed "s:/run/secrets/outreach.io:$configDir:g" |
      sed "s:/run/volumes/outreach.io:$volumeDir:g" >"$tmpFile"

    # If the file already exists, then merge it with the new one.
    # NOTE: We probably want to make this smarter and log when merging. I don't think
    # that is feasible to do in bash, though....
    if [[ -e $saveFile ]] && [[ ${saveFile##*.} =~ ^(yaml|yml)$ ]]; then
      info_sub "$configFile (merged)"
      yq --yaml-output -s '.[0] * .[1]' "$tmpFile" "$saveFile" >"$mergedFile"
      mv "$mergedFile" "$saveFile"
    else
      info_sub "$configFile"
      mv "$tmpFile" "$saveFile"
    fi

    rm "$tmpFile" "$mergedFile" >/dev/null 2>&1 || true
  done

# Fetch secrets from Vault and store them at ~/.outreach/<appName>
# In Kubernetes these will be stored in the same format, but at the path
# /run/secrets/outreach.io/<basename vaultKey>/<vault subKey>
info "Fetching Secret(s) from Vault"

# DEVENV_VERSION is set to get around the deprecation for now. The script is deprecated for normal users
# but is still needed for rendering manifests.
DEVENV_VERSION="xyz" "$DIR/deploy-to-dev.sh" show | yq -r 'select(.kind == "VaultSecret") | .spec.path' |
  while IFS=$'\n' read -r vaultKey; do
    info_sub "$vaultKey"
    get_vault_secrets "$vaultKey" "$HOME/.outreach/$APPNAME"
  done
info_sub "fetching secret 'dev/devenv/honeycomb'"
get_vault_secrets "dev/devenv/honeycomb" "$configDir"

# We add logfmt.yaml directly here because this is only needed for local development.
# This is not meant to be used in any kubernetes setup
info "Configuring logfmt"
mkdir -p "$HOME/.outreach/logfmt"
get_vault_secrets "dev/datadog/dev-env" "$HOME/.outreach/logfmt"
cat >"$HOME/.outreach/logfmt/logfmt.yaml" <<EOF
DatadogAPIKey:
  Path: "$HOME/.outreach/logfmt/dev-env/api_key"
EOF

# Look for a override script that allows users to extend this process outside of bootstrap
if [[ -e $overridePath ]]; then
  # shellcheck disable=SC1090
  source "$overridePath"
fi
