#!/usr/bin/env bash
# Configures a CI machine to run a devenv instance suitable for E2E testing
set -e

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"

# shellcheck source=../../lib/logging.sh
source "$DIR/../../lib/logging.sh"

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
    sudo curl -fsSLo /usr/share/keyrings/kubernetes-archive-keyring.gpg https://packages.cloud.google.com/apt/doc/apt-key.gpg
    echo "deb [signed-by=/usr/share/keyrings/kubernetes-archive-keyring.gpg] https://apt.kubernetes.io/ kubernetes-xenial main" | sudo tee /etc/apt/sources.list.d/kubernetes.list
    sudo apt-get update -y
    sudo apt-get install -y kubectl
  fi

  if ! command -v kubecfg >/dev/null; then
    info "Installing kubecfg"
    curl -fsSL https://github.com/getoutreach/kubecfg/releases/download/v0.17.0/kubecfg-linux-amd64 >kubecfg
    chmod +x kubecfg
    sudo mv kubecfg /usr/local/bin/kubecfg
  fi

  if ! command -v devenv >/dev/null; then
    info "Setting up devenv"

    tempDir=$(mktemp -d)
    cp "$DIR/../../../.tool-versions" "$tempDir/" # Use the versions from devbase
    pushd "$tempDir" >/dev/null || exit 1
    # Ensure the versions we need are available
    asdf install

    # download the pre-release/latest version
    REPO=getoutreach/devenv
    if [[ $DEVENV_PRE_RELEASE == "true" ]]; then
      TAG=$(gh release -R "$REPO" list | grep Pre-release | head -n1 | awk '{ print $1 }')
    else
      TAG=$(gh release -R "$REPO" list | grep Latest | awk '{ print $1 }')
    fi
    info "Using devenv version: ($TAG)"
    gh release -R "$REPO" download "$TAG" --pattern "devenv_*_$(go env GOOS)_$(go env GOARCH).tar.gz"

    echo "" # Fixes issues with output being corrupted in CI
    tar xf devenv**.tar.gz
    sudo mv devenv /usr/local/bin/devenv
    sudo chown circleci:circleci /usr/local/bin/devenv
    rm -rf "$tempDir"
    popd >/dev/null || exit
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
    # Use the CI vault instance.
    # TODO(jaredallard): Refactor when using box is available to CI.
    export VAULT_ADDR="https://vault-dev.outreach.cloud"
  fi

  info "Provisioning developer environment"
  # shellcheck disable=SC2086 # Why: Not an array, have to split.
  exec devenv --skip-update provision $PROVISION_ARGS
fi

if [[ $E2E == "true" ]]; then
  info "Starting E2E test runner"
  TEST_TAGS=or_test,or_e2e exec "$("$DIR/../../gobin.sh" -p "github.com/getoutreach/devbase/v2/e2e@$(cat "$DIR/../../../.version")")"
fi
