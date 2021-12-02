#!/usr/bin/env bash
# Sets up Github Packages for a variety of languages.

if [[ -z $GITHUB_TOKEN ]] || [[ -z $GITHUB_USERNAME ]]; then
  echo "Skipping: GITHUB_TOKEN or GITHUB_USERNAME is not set."
  exit 0
fi

# TODO: Replace this with box config when CI has access.
ORG=getoutreach

# Setup Ruby Authentication if bundle exists.
if command -v bundle >/dev/null 2>&1; then
  # Configure bundler access
  bundle config "https://rubygems.pkg.github.com/$ORG" "$GITHUB_USERNAME:$GITHUB_TOKEN"

  # Configure gem access
  mkdir -p "$HOME/.gem"
  cat >"$HOME/.gem/credentials" <<EOF
---
:github: Bearer $GITHUB_TOKEN
EOF

  chmod 0600 "$HOME/.gem/credentials"

  cat >"$HOME/.gemrc" <<EOF
---
:backtrace: false
:bulk_threshold: 1000
:sources:
- https://rubygems.org/
- https://$GITHUB_USERNAME:$GITHUB_TOKEN@rubygems.pkg.github.com/$ORG
:update_sources: true
:verbose: true  
EOF
fi

if command -v npm >/dev/null 2>&1; then
  # Do not remove the empy newline, this ensures we never write to the same line
  # as something else.
  cat >>"$HOME/.npmrc" <<EOF

//npm.pkg.github.com/:_authToken=$GITHUB_TOKEN
EOF
fi
