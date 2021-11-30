#!/usr/bin/env bash
# Sets up most standard requirements for CI
set -e

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
CI_AUTH_DIR="$DIR/../ci/auth"

authn=(
  "npm"
  "ssh"
  "gcr"
  "vault"
  "aws"
  "github"
  "packagecloud"
  "github_packages"
)

for authName in "${authn[@]}"; do
  echo "🔒 Setting up $authName access"
  "$CI_AUTH_DIR/$authName.sh"
done

# Setup $TEST_RESULTS if it's set
if [[ -n $TEST_RESULTS ]]; then
  mkdir -p "$TEST_RESULTS"
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

# Setup a cache-version.txt file that can be used to invalidate cache via env vars in CircleCI
echo "$CACHE_VERSION" >>cache-version.txt
