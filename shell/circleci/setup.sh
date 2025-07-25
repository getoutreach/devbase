#!/usr/bin/env bash
# Sets up most standard requirements for CI
set -e

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
CI_DIR="$DIR/../ci"
LIB_DIR="$DIR/../lib"

# shellcheck source=../lib/logging.sh
source "${LIB_DIR}/logging.sh"

# shellcheck source=../lib/mise.sh
source "${LIB_DIR}/mise.sh"

# shellcheck source=../lib/shell.sh
source "${LIB_DIR}/shell.sh"

if [[ -z $ALLOW_MISE_TO_MANAGE_TOOL_VERSIONS ]]; then
  # Ensure that asdf is ready to be used
  info "🔨 Setting up asdf"
  "$CI_DIR/env/asdf.sh"
fi

info "🔨 Setting up mise 🧑‍🍳"
ensure_mise_installed

"$CI_DIR/env/mise.sh"

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

# Setup a cache-version.txt file that can be used to invalidate cache via env vars in CircleCI
echo "$CACHE_VERSION" >cache-version.txt

# Authenticate with AWS ECR now that we have the box config
info "🔒 Setting up AWS ECR access"

# shellcheck source=../lib/docker.sh
source "${LIB_DIR}/docker.sh"

if [[ -z $DOCKER_PUSH_REGISTRIES ]]; then
  DOCKER_PUSH_REGISTRIES="$(get_docker_push_registries)"
fi
# shellcheck source=../ci/auth/aws-ecr.sh
"$CI_DIR/auth/aws-ecr.sh"
