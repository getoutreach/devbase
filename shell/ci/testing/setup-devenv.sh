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
    curl -fsSL https://github.com/getoutreach/kubecfg/releases/download/v0.17.0/kubecfg-linux-amd64 >"/usr/local/bin/kubecfg"
    chmod +x /usr/local/bin/kubecfg
  fi

  if ! command -v devenv >/dev/null; then
    info "Setting up devenv"

    tempDir=$(mktemp -d)
    pushd "$tempDir" >/dev/null || exit 1
    gh release -R getoutreach/devenv download --pattern "devenv_*_$(go env GOOS)_$(go env GOARCH).tar.gz"
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
  info "Provisioning developer environment"
  exec devenv provision
fi

if [[ $E2E == "true" ]]; then
  info "Starting E2E test runner"
  exec "$("$DIR/gobin.sh" -p "github.com/getoutreach/devbase/e2e@$(cat "$DIR/../.version")")"
fi
