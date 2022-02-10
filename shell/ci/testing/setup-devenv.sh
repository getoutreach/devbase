#!/usr/bin/env bash
# Configures a CI machine to run a devenv instance suitable for E2E testing
set -e

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"

# shellcheck source=../../lib/logging.sh
source "$DIR/../../lib/logging.sh"

# Arguments
PROVISION="${PROVISION:-"false"}"
E2E="${E2E:-"false"}"

if [[ $PROVISION == "true" ]] && [[ $E2E == "true" ]]; then
  info "e2e was set, ignoring provision"
  PROVISION="false"
fi

if [[ -z $VAULT_ROLE_ID ]]; then
  echo "Hint: Outreach CircleCI must be configured to have"
  echo "  vault-dev be added to the list of contexts for this"
  echo "  CircleCI workflow"
  fatal "Vault must be configured to setup a devenv"
fi

if [[ -z $AWS_ACCESS_KEY_ID ]]; then
  echo "Hint: Outreach CircleCI must be configured to have"
  echo "  aws-credentials be added to the list of contexts for this"
  echo "  CircleCI workflow"
  fatal "Vault must be configured to setup a devenv"
fi

# CI sets up dependencies in CI and other small adjustments.
# These are not required on local machines.
if [[ -n $CI ]]; then
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

    # If we're in the devenv repo, use a local build.
    if [[ "$(yq -r '.name' service.yaml)" == "devenv" ]]; then
      info_sub "Using local devenv build"
      warn "Note: snapshot downloader is currently unable to be tested"
      set -x
      make "APP_VERSION=$(git describe --tags --abbrev=0)"
      set +x
      info_sub "built binary: $(./bin/devenv --version)"
      sudo cp ./bin/devenv /usr/local/bin/devenv
    else
      tempDir=$(mktemp -d)
      cp "$DIR/../../../.tool-versions" "$tempDir/" # Use the versions from devbase
      pushd "$tempDir" >/dev/null || exit 1
      gh release -R getoutreach/devenv download --pattern "devenv_*_$(go env GOOS)_$(go env GOARCH).tar.gz"
      echo "" # Fixes issues with output being corrupted in CI
      tar xf devenv**.tar.gz
      sudo mv devenv /usr/local/bin/devenv
      sudo chown circleci:circleci /usr/local/bin/devenv
      rm -rf "$tempDir"
      popd >/dev/null || exit
    fi
  fi

  info "Setting up Git"
  git config --global user.name "CircleCI E2E Test"
  git config --global user.email "circleci@outreach.io"
fi

if [[ $PROVISION == "true" ]]; then
  info "Checking for existing devenv ..."
  if devenv --skip-update status >/dev/null; then
    info "Using already provisioned developer environment"
    exit 0
  fi

  # Use the CI vault instance.
  # TODO(jaredallard): Refactor when using box is available to CI.
  export VAULT_ADDR="https://vault-dev.outreach.cloud"

  info "Provisioning developer environment"
  exec devenv --skip-update provision
fi

if [[ $E2E == "true" ]]; then
  info "Starting E2E test runner"
  exec "$("$DIR/gobin.sh" -p "github.com/getoutreach/devbase/e2e@$(cat "$DIR/../.version")")"
fi
