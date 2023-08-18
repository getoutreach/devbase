#!/usr/bin/env bash
# Runs specific tests for parts of devbase that aren't just Go.

echo "Running bats tests..."
SCRIPTS_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
"${SCRIPTS_DIR}/bash-test-runner.sh" || exit 1

echo "Linting devbase orb"
if ! command -v circleci &>/dev/null; then
  echo "circleci command not found. Please install the CircleCI CLI to run this test." >&2
  echo "Hint: brew install circleci" >&2
  exit 1
fi
make validate-orb || exit 1
