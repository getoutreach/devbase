#!/usr/bin/env bash
# Sets up most standard requirements for CI
set -e

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
CI_DIR="$DIR/../ci"
LIB_DIR="$DIR/../lib"

# Ensure that asdf is ready to be used
echo "🔨 Setting up asdf"
"$CI_DIR/env/asdf.sh"

authn=(
  "npm"
  "ssh"
  "gcr"
  "vault"
  "aws"
  "github"
  "github_packages"
)

for authName in "${authn[@]}"; do
  echo "🔒 Setting up $authName access"
  "$CI_DIR/auth/$authName.sh"
done

# Setup $TEST_RESULTS if it's set
if [[ -n $TEST_RESULTS ]]; then
  mkdir -p "$TEST_RESULTS"
fi

# run prescript if user specified to install packages etc. before tests
if [[ -n $PRE_SETUP_SCRIPT ]]; then
  echo "⚙️ Running setup script \"${PRE_SETUP_SCRIPT}\" (from pre_setup_script)"
  # shellcheck source=/dev/null
  "${PRE_SETUP_SCRIPT}"
fi

# Setup a box stub
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

# Setup a cache-version.txt file that can be used to invalidate cache via env vars in CircleCI
echo "$CACHE_VERSION" >cache-version.txt
