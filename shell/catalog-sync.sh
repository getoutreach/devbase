#!/usr/bin/env bash
#
# DEPRECATED: Use `mise run stencil:post:catalog-sync` instead.
#
# Syncs the service catalog manifest for the given repository with
# the metadata present in the repository.

set -euo pipefail

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"

# shellcheck source=./lib/logging.sh
source "$DIR/lib/logging.sh"

warn "This script is deprecated and will be removed in the future. Please use 'mise run stencil:post:catalog-sync' instead."
echo
warn "Starting in 5 seconds..."
echo

sleep 5

exec mise run stencil:post:catalog-sync "$@"
