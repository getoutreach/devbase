#!/usr/bin/env bash
# This script exists to run multiple linters for vscode acting
# as a single one, to get the messages to show up in VSCode.

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"

# forward signals to child processes
trap 'kill $(jobs -p)' EXIT

# run in background so we can forward signals
"$DIR/../lintroller.sh" ./... &

# Wait for lintroller to finish
wait

exec "$DIR/../golangci-lint.sh" "$@"
