#!/usr/bin/env bash
# Sets up Github Packages for a variety of languages.
DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
LIB_DIR="${DIR}/../../lib"

# shellcheck source=../../lib/bootstrap.sh
source "${LIB_DIR}/bootstrap.sh"

# shellcheck source=../../lib/github.sh
source "${LIB_DIR}/github.sh"

# shellcheck source=../../lib/logging.sh
source "${LIB_DIR}/logging.sh"

# shellcheck source=../../lib/docker/authn/ghcr.sh
source "${LIB_DIR}/docker/authn/ghcr.sh"

# Can't use app token for GitHub Packages
GITHUB_PACKAGES_TOKEN="$(fetch_github_token_from_ci --env-prefix GHACCESSTOKEN_PAT)"

# Allow setting for using static auth
if [[ -z ${GITHUB_USERNAME:-} ]]; then
  # See: https://docs.github.com/en/developers/apps/building-github-apps/authenticating-with-github-apps#http-based-git-access-by-an-installation
  GITHUB_USERNAME="x-access-token"
fi

# TODO: Replace this with box config when CI has access.
ORG=getoutreach

# Setup Ruby Authentication if bundle exists.
if command -v bundle >/dev/null 2>&1; then
  info_sub "💎 Ruby"
  # Configure bundler access
  bundle config "https://rubygems.pkg.github.com/$ORG" "$GITHUB_USERNAME:$GITHUB_PACKAGES_TOKEN"

  # Configure gem access
  mkdir -p "$HOME/.gem"
  cat >"$HOME/.gem/credentials" <<EOF
---
:github: Bearer $GITHUB_PACKAGES_TOKEN
EOF

  chmod 0600 "$HOME/.gem/credentials"

  cat >"$HOME/.gemrc" <<EOF
---
:backtrace: false
:bulk_threshold: 1000
:sources:
- https://rubygems.org/
- https://$GITHUB_USERNAME:$GITHUB_PACKAGES_TOKEN@rubygems.pkg.github.com/$ORG
:update_sources: true
:verbose: true
EOF
fi

if command -v npm >/dev/null 2>&1; then
  info_sub "Node.js"
  # Do not remove the empty newline, this ensures we never write to the same line
  # as something else.
  cat >>"$HOME/.npmrc" <<EOF

//npm.pkg.github.com/:_authToken=$GITHUB_PACKAGES_TOKEN
@$ORG:registry=https://npm.pkg.github.com
EOF
fi

info_sub "Docker"

ghcr_auth "$ORG"
