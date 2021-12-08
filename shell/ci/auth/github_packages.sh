#!/usr/bin/env bash
# Sets up Github Packages for a variety of languages.

tokenFile="$HOME/.outreach/github.token"

if [[ ! -e $tokenFile ]]; then
  echo "Skipped: Github Auth not setup"
fi

GH_TOKEN="$(cat "$tokenFile")"

# Allow setting for using static auth
if [[ -z $GITHUB_USERNAME ]]; then
  # See: https://docs.github.com/en/developers/apps/building-github-apps/authenticating-with-github-apps#http-based-git-access-by-an-installation
  GITHUB_USERNAME="x-access-token"
fi

# TODO: Replace this with box config when CI has access.
ORG=getoutreach

# Setup Ruby Authentication if bundle exists.
if command -v bundle >/dev/null 2>&1; then
  # Configure bundler access
  bundle config "https://rubygems.pkg.github.com/$ORG" "$GITHUB_USERNAME:$GH_TOKEN"

  # Configure gem access
  mkdir -p "$HOME/.gem"
  cat >"$HOME/.gem/credentials" <<EOF
---
:github: Bearer $GH_TOKEN
EOF

  chmod 0600 "$HOME/.gem/credentials"

  cat >"$HOME/.gemrc" <<EOF
---
:backtrace: false
:bulk_threshold: 1000
:sources:
- https://rubygems.org/
- https://$GITHUB_USERNAME:$GH_TOKEN@rubygems.pkg.github.com/$ORG
:update_sources: true
:verbose: true
EOF
fi

if command -v npm >/dev/null 2>&1; then
  # Do not remove the empy newline, this ensures we never write to the same line
  # as something else.
  cat >>"$HOME/.npmrc" <<EOF

//npm.pkg.github.com/:_authToken=$GH_TOKEN
EOF
fi
