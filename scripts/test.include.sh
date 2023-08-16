#!/usr/bin/env bash
# Runs bats, see bash-test-runner.sh for more details.

echo "Running bats tests..."
SCRIPTS_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
"${SCRIPTS_DIR}/bash-test-runner.sh" || exit 1
