#!/usr/bin/env bash
#
# GHCR (GitHub Container Registry) authentication.

set -eo pipefail

# ghcr_auth(org)
# Assumes that GITHUB_TOKEN has been set already with a non-GitHub
# App-based token, usually via bootstrap_github_token from
# `shell/lib/github.sh`.
ghcr_auth() {
  local org="$1"

  echo "$GITHUB_TOKEN" |
    docker login ghcr.io --username="$org" --password-stdin
}
