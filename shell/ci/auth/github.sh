#!/usr/bin/env bash
set -e

# DEPRECATED: Replaced with GITHUB_TOKEN in the github-credentials context
# for Outreach.
if [[ -n $OUTREACH_GITHUB_TOKEN ]]; then
  GITHUB_TOKEN="$OUTREACH_GITHUB_TOKEN"
fi

if [[ -z $GITHUB_TOKEN ]]; then
  echo "Skipped: GITHUB_TOKEN is not set."
  exit 0
fi

mkdir -p "$HOME/.outreach"
echo "$GITHUB_TOKEN" >"$HOME/.outreach/github.token"
