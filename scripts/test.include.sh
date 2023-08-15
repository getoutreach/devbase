#!/usr/bin/env bash
# Runs bats, see bash-test-runner.sh for more details.

echo "Running bats tests..."
DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
"${DIR}/bash-test-runner.sh" || exit 1
