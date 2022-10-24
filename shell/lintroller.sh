#!/usr/bin/env bash
# Runs lintroller
set -e -o pipefail

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"

# shellcheck source=./lib/bootstrap.sh
source "$DIR/lib/bootstrap.sh"

export GOFLAGS=-tags=or_e2e,or_test

if [[ -z $workspaceFolder ]]; then
  workspaceFolder="$(get_repo_directory)"
fi

# The sed is used to strip the pwd from lintroller output,
# which is currently prefixed with it.
exec "$DIR/gobin.sh" \
  "github.com/getoutreach/lintroller/cmd/lintroller@v$(get_application_version "lintroller")" \
  -config "$workspaceFolder/scripts/golangci.yml" "$@" 2>&1 | sed "s#^$(pwd)/##"
