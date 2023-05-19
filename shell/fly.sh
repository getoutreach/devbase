#!/usr/bin/env bash

set -e -o pipefail

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"

# shellcheck source=./lib/logging.sh
source "$DIR/lib/logging.sh"

if [[ "$(uname)" == "Darwin" ]]; then
  if ! brew list fly >/dev/null 2>&1; then
    brew install fly
    xattr -c "$(brew --prefix)"/bin/fly >/dev/null 2>&1
  fi
elif ! command -v fly >/dev/null; then
  error "fly not found, re-run 'orc setup'"
  exit 1
fi

fly "$@"
