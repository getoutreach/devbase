#!/usr/bin/env bash
# Configures a CI machine to run a devenv instance suitable for E2E testing
set -e

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"

# shellcheck source=../../lib/logging.sh
source "$DIR/../../lib/logging.sh"
# shellcheck source=../../lib/github.sh
source "$DIR/../../lib/github.sh"
# shellcheck source=../../lib/box.sh
source "$DIR/../../lib/box.sh"

# Arguments
PROVISION="${PROVISION:-"false"}"
PROVISION_ARGS="${PROVISION_ARGS:-""}"
E2E="${E2E:-"false"}"

if [[ $PROVISION == "true" ]] && [[ $E2E == "true" ]]; then
  info "e2e was set, ignoring provision"
  PROVISION="false"
fi

# CI sets up dependencies in CI and other small adjustments.
# These are not required on local machines.
if [[ -n $CI ]]; then
  if [[ -z $VAULT_ROLE_ID ]]; then
    echo "Hint: Outreach CircleCI must be configured to have"
    echo "  vault-dev be added to the list of contexts for this"
    echo "  CircleCI workflow"
    fatal "Vault must be configured to setup a devenv"
  fi

  if ! command -v kubectl >/dev/null; then
    info "Installing kubectl"
    tmpFile=$(mktemp)
    keyringLocation=/etc/apt/keyrings/kubernetes-archive-keyring.gpg
    k8sAptSourceLocation="https://pkgs.k8s.io/core:/stable:/v1.29/deb/"

    curl -fsSLo "$tmpFile" "${k8sAptSourceLocation}Release.key"

    # 2023-05-23: GCP changed their key to be armored and changed the key location. Currently
    # the file extension is incorrect. So, we handle if it is armored or not in case
    # they end up fixing it.
    if grep -q -- "-----BEGIN" "$tmpFile"; then
      # Output is armored, convert to binary
      gpg --output - --dearmor "$tmpFile" | sudo tee "$keyringLocation" >/dev/null
      rm "$tmpFile"
    else
      echo "Warning: GCP apt-key is not armored. We can remove this workaround now."
      # Otherwise, just copy the file
      sudo cp "$tmpFile" "$keyringLocation"
    fi
    echo "deb [signed-by=$keyringLocation] ${k8sAptSourceLocation} /" | sudo tee /etc/apt/sources.list.d/kubernetes.list >/dev/null
    sudo apt-get update -y
    sudo apt-get install -y kubectl
  fi

  if ! command -v kubecfg >/dev/null; then
    info "Installing kubecfg"
    curl -fsSL https://github.com/getoutreach/kubecfg/releases/download/v0.28.1/kubecfg-linux-amd64 >kubecfg
    chmod +x kubecfg
    sudo mv kubecfg /usr/local/bin/kubecfg
  fi

  if ! command -v devenv >/dev/null; then
    install_latest_github_release getoutreach/devenv "$DEVENV_PRE_RELEASE"
  fi

  info "Setting up Git"
  git config --global user.name "CircleCI E2E Test"
  git config --global user.email "circleci@outreach.io"
fi

if [[ $PROVISION == "true" ]]; then
  info "Checking for existing devenv ..."
  if devenv --skip-update status >/dev/null 2>&1; then
    info "Using already provisioned developer environment"
    exit 0
  fi

  if [[ -n $CI ]]; then
    if [[ -z $VAULT_ADDR ]]; then
      VAULT_ADDR="$(get_box_field devenv.vault.address)"
      export VAULT_ADDR
    fi
  fi

  info "Provisioning developer environment"
  # shellcheck disable=SC2086 # Why: Not an array, have to split.
  exec devenv --skip-update provision $PROVISION_ARGS
fi

if [[ $E2E == "true" ]]; then
  info "Starting E2E test runner"
  TEST_TAGS=or_test,or_e2e exec "$("$DIR/../../gobin.sh" -p "github.com/getoutreach/devbase/v2/e2e@$(cat "$DIR/../../../.version")")"
fi
