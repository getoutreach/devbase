#!/usr/bin/env bash
# Runs e2e tests for the service in the current directory
DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"

if [[ $CI == "true" ]]; then
  "$DIR/circleci/setup-e2e-deps.sh"
  "$DIR/circleci/setup-vault.sh"
  "$DIR/circleci/setup-e2e.sh"
fi

exec "$("$DIR/gobin.sh" -p "github.com/getoutreach/devbase/e2e@$(cat "$DIR/../.version")")"
