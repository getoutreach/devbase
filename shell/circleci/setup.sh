#!/usr/bin/env bash
# Sets up most standard requirements for CI
set -e

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
CI_DIR="$DIR/../ci"
LIB_DIR="$DIR/../lib"

# shellcheck source=../lib/bootstrap.sh
source "${LIB_DIR}/bootstrap.sh"

# shellcheck source=../lib/circleci.sh
source "${LIB_DIR}/circleci.sh"

# shellcheck source=../lib/github.sh
source "${LIB_DIR}/github.sh"

# shellcheck source=../lib/logging.sh
source "${LIB_DIR}/logging.sh"

# shellcheck source=../lib/mise.sh
source "${LIB_DIR}/mise.sh"

# shellcheck source=../lib/shell.sh
source "${LIB_DIR}/shell.sh"

# shellcheck source=../lib/rate_limit.sh
source "${LIB_DIR}/rate_limit.sh"

# shellcheck source=../lib/version.sh
source "${LIB_DIR}/version.sh"

info "🔨 Setting up mise 🧑‍🍳"
ensure_mise_installed

if gh_installed; then
  # shellcheck disable=SC2119
  # Why: no extra args needed to pass to ghaccesstoken in this case.
  bootstrap_github_token
fi

if ! mise_manages_tool_versions; then
  # Ensure that asdf is ready to be used
  info "🔨 Setting up asdf"
  "$CI_DIR/env/asdf.sh"
fi

"$CI_DIR/env/mise.sh"

if circleci_pr_is_fork; then
  warn "🔒 🙅 NOT Setting up authentication, as this PR is from a fork"
else
  authn=(
    "npm"
    "ssh"
    "aws"
    "github"
    "github_packages"
  )

  for authName in "${authn[@]}"; do
    info "🔒 Setting up $authName access"
    "$CI_DIR/auth/$authName.sh"
  done

  # Log GitHub API rate limit after auth setup for observability.
  # resolve_github_token prefers the PAT from ~/.npmrc (written by
  # github_packages.sh) over GITHUB_TOKEN, avoiding an extra
  # ghaccesstoken invocation (which costs 2 API calls).
  read -r _rl_token _rl_source <<<"$(resolve_github_token)"
  if [[ -n $_rl_token ]]; then
    log_github_rate_limit "$_rl_token" "setup_end" "$_rl_source"
  fi
  unset _rl_token _rl_source
fi

# Setup $TEST_RESULTS if it's set
if [[ -n $TEST_RESULTS ]]; then
  mkdir -p "$TEST_RESULTS"
fi

# run pre-script if user specified to install packages etc. before tests
if [[ -n $PRE_SETUP_SCRIPT ]]; then
  info "⚙️ Running setup script \"${PRE_SETUP_SCRIPT}\" (from pre_setup_script)"
  # shellcheck source=/dev/null
  "${PRE_SETUP_SCRIPT}"
fi

# Setup a cache-version.txt file that can be used to invalidate cache via env vars in CircleCI
echo "$CACHE_VERSION" >cache-version.txt

# Setup box config and Vault/ECR access if not a fork PR
if ! circleci_pr_is_fork; then
  # Set up a box stub
  boxPath="$HOME/.outreach/.config/box/box.yaml"
  mkdir -p "$(dirname "$boxPath")"
  cat >"$boxPath" <<EOF
lastUpdated: 2021-01-01T00:00:00.0000000Z
config:
  refreshInterval: 0s
  devenv:
    vault: {}
    runtimeConfig: {}
    snapshots: {}
storageURL: git@github.com:getoutreach/box
EOF

  # shellcheck source=../lib/box.sh
  source "${LIB_DIR}/box.sh"

  # Ensure we have the latest box config
  download_box

  # Authenticate with Vault now that we have the box config
  info "🔒 Setting up Vault access"

  # shellcheck source=../ci/auth/vault.sh
  "$CI_DIR/auth/vault.sh"

  # Authenticate with AWS ECR now that we have the box config
  info "🔒 Setting up AWS ECR access"

  # shellcheck source=../lib/docker.sh
  source "${LIB_DIR}/docker.sh"

  if [[ -z $DOCKER_PUSH_REGISTRIES ]]; then
    DOCKER_PUSH_REGISTRIES="$(get_docker_push_registries)"
  fi
  # shellcheck source=../ci/auth/aws-ecr.sh
  "$CI_DIR/auth/aws-ecr.sh"
fi
