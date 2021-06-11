#!/usr/bin/env bash
# Runs e2e tests for the service in the current directory
DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"

if [[ $CI == "true" ]]; then
  "$DIR/circleci/setup-vault.sh"
  "$DIR/circleci/setup-e2e.sh"

  # Bootstrap puts this here. We could def make this better.
  docker exec -it devenv /host_mnt/scripts/shell-wrapper.sh gobin.sh "github.com/getoutreach/devbase/e2e@$(cat "$DIR/../../.version")"
fi

"$DIR/gobin.sh" "github.com/getoutreach/devbase/e2e@$(cat "$DIR/../../.version")"
