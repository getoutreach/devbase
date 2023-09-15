#!/usr/bin/env bash
# Publishes an orb whenever an unstable release is created.

if ! command -v circleci &>/dev/null; then
  echo "Error: circleci CLI not found, please install it." >&2
  exit 1
fi

# If we're not logged in, attempt to login in CI or fail locally.
if ! circleci info org >/dev/null; then
  if [[ $CIRCLECI != "true" ]]; then
    echo "Error: CircleCI not setup locally, please ensure the 'circleci'" >&2
    echo "CLI is installed and that 'circleci setup' has been ran." >&2
    exit 1
  fi

  # In CI, use the well-known "$CIRCLECI_API_TOKEN" env var to configure
  # the CLI to use the correct token.
  #
  # Note: This comes from the circleci-credentials context.
  circleci setup --no-prompt --host "https://circleci.com" --token "$CIRCLECI_API_TOKEN"
fi

if [[ $DRYRUN == true ]]; then
  echo "Skipping orb publish due to --dry-run flag"
  exit 0
fi

exec make publish-orb
