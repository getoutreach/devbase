#!/usr/bin/env bash
# Publishes an orb whenever an unstable release is created.

# If we're not logged in, attempt to login in CI or fail locally.
if ! circleci info org >/dev/null; then
  if [[ $CIRCLECI != "true" ]]; then
    echo "CircleCI not setup locally, please ensure the 'circleci' CLI is"
    echo "installed and that 'circleci setup' has been ran."
    exit 1
  fi

  # In CI, use the well-known "$CIRCLECI_API_TOKEN" env var to configure
  # the CLI to use the correct token.
  #
  # Note: This comes from the circleci-credentials context.
  circleci setup --no-prompt --host "https://circleci.com" --token "$CIRCLECI_API_TOKEN"
fi

exec make publish-orb
