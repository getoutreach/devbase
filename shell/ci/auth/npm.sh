#!/usr/bin/env bash
# DEPRECATED: Use Github Packages instead
# Setup NPM authentication
set -e

if [[ -z $NPM_TOKEN ]]; then
  echo "Skipped: NPM_TOKEN is not set."
  exit 0
fi

echo "//registry.npmjs.org/:_authToken=$NPM_TOKEN" >>~/.npmrc
