#!/usr/bin/env bash
set -e

mkdir -p "$HOME/.outreach"
echo "$OUTREACH_GITHUB_TOKEN" >"$HOME/.outreach/github.token"
