#!/usr/bin/env bash
#
# DEPRECATED: Use `mise run stencil:post:circleci-orb-sync` instead.
#
# Syncs the CircleCI orb definition with the version of devbase in the
# stencil.lock file.  By default, it only updates .circleci/config.yml
# (the default config file), but this is only necessary for repositories
# which do not use stencil-circleci to manage the CircleCI config.
# The default config file is validated, others are not (as they may not
# be config files per se, such as orb definitions).

set -euo pipefail

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"

# shellcheck source=./lib/logging.sh
source "$DIR/lib/logging.sh"

warn "This script is deprecated and will be removed in the future. Please use 'mise run stencil:post:circleci-orb-sync' instead."

exec mise run stencil:post:circleci-orb-sync "$@"
