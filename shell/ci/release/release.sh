#!/usr/bin/env bash
# Release some code

# Unset NPM_TOKEN to force it to use the configured ~/.npmrc
NPM_TOKEN='' GH_TOKEN=$GITHUB_TOKEN \
  yarn --frozen-lockfile semantic-release
