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
# shellcheck source=../../lib/mise.sh
source "$DIR/../../lib/mise.sh"
# shellcheck source=../../lib/shell.sh
source "$DIR/../../lib/shell.sh"
# shellcheck source=../../lib/version.sh
source "$DIR/../../lib/version.sh"

# Arguments
PROVISION="${PROVISION:-"false"}"
PROVISION_ARGS="${PROVISION_ARGS:-""}"
# E2E is this script's behavior flag (set by `make e2e` or the orb's
# setup_devenv command): when true, this script asserts E2E tools on
# PATH and runs the test suite. Distinct from INSTALL_E2E_TOOLS, which
# is the CI-bootstrap flag (see shell/circleci/machine.sh,
# shell/ci/env/mise.sh) that picks mise.e2e.toml over mise.devbase.toml
# before this script runs.
E2E="${E2E:-"false"}"
DEVENV_PRE_RELEASE="${DEVENV_PRE_RELEASE:-"false"}"

if [[ $PROVISION == "true" ]] && [[ $E2E == "true" ]]; then
  info "e2e was set, ignoring provision"
  PROVISION="false"
fi

# CI sets up dependencies in CI and other small adjustments.
# These are not required on local machines.
if in_ci_environment; then
  if [[ -z $VAULT_ROLE_ID ]]; then
    echo "Hint: Outreach CircleCI must be configured to have"
    echo "  vault-dev be added to the list of contexts for this"
    echo "  CircleCI workflow"
    fatal "Vault must be configured to setup a devenv"
  fi

  mise_path="$(find_mise)"
  eval "$("$mise_path" activate bash --shims)"

  # In E2E mode, setup_environment installs kubectl/kubecfg/devenv from
  # mise.e2e.lock before this script runs. If any are missing, the
  # lockfile-based install drifted from this script's expectations;
  # fail loudly rather than silently falling through to unpinned
  # installs below.
  if [[ $E2E == "true" ]]; then
    for tool in kubectl kubecfg devenv; do
      if ! command_exists "$tool"; then
        fatal "$tool not on PATH in E2E mode; expected setup_environment to install it from mise.e2e.lock"
      fi
    done
  fi

  if ! command_exists kubectl; then
    install_tool_with_mise kubectl "$(tool_version_from_mise_env e2e kubectl)"
  fi

  if ! command_exists kubecfg; then
    install_tool_with_mise github:getoutreach/kubecfg "$(tool_version_from_mise_env e2e github:getoutreach/kubecfg)"
  fi

  if ! command_exists devenv; then
    if [[ $DEVENV_PRE_RELEASE == "true" ]]; then
      install_latest_github_release getoutreach/devenv "$DEVENV_PRE_RELEASE"
    else
      install_tool_with_mise github:getoutreach/devenv "$(tool_version_from_mise_env e2e github:getoutreach/devenv)"
    fi
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

  if in_ci_environment && [[ -z $VAULT_ADDR ]]; then
    VAULT_ADDR="$(get_box_field devenv.vault.address)"
    export VAULT_ADDR
  fi

  info "Provisioning developer environment"
  # shellcheck disable=SC2086 # Why: Not an array, have to split.
  exec devenv --skip-update provision $PROVISION_ARGS
fi

if [[ $E2E == "true" ]]; then
  info "Starting E2E test runner"
  TEST_TAGS=or_test,or_e2e exec "$("$DIR/../../gobin.sh" -p "github.com/getoutreach/devbase/v2/e2e@$(cat "$DIR/../../../.version")")"
fi
