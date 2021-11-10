#!/usr/bin/env bash
# Runs e2e tests for the service in the current directory
set -e

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"

# shellcheck source=./lib/logging.sh
source "$DIR/lib/logging.sh"

if [[ $CI == "true" ]]; then
  sudo "$DIR/ci/e2e/setup-e2e-deps.sh"
  "$DIR/ci/e2e/setup-e2e.sh"
fi

info "Starting E2E runner"
exec "$("$DIR/gobin.sh" -p "github.com/getoutreach/devbase/e2e@$(cat "$DIR/../.version")")"
