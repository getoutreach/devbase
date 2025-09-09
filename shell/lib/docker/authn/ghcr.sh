#!/usr/bin/env bash
#
# GHCR (GitHub Container Registry) authentication.

set -eo pipefail

# ghcr_auth(org)
# Assumes that GITHUB_TOKEN has been set already.
ghcr_auth() {
  local org="$1"

  echo "$GITHUB_TOKEN" |
    docker login ghcr.io --username="$org" --password-stdin
}
