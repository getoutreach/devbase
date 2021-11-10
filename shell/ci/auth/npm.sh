#!/usr/bin/env bash
# Setup NPM authentication
set -e

echo "//registry.npmjs.org/:_authToken=$NPM_TOKEN" >>~/.npmrc
