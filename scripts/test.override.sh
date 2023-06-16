#!/usr/bin/env bash
# Calls extra test functions
set -euo pipefail
DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
exec "${DIR}/bash-test-runner.sh"
