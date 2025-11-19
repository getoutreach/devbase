#!/usr/bin/env bash
# Sets up SSH authentication in CI

set -eo pipefail

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
LIB_DIR="${DIR}/../../lib"

# shellcheck source=../../lib/logging.sh
source "$LIB_DIR/logging.sh"

if [[ -f ~/.ssh/config ]]; then
  # Setup SSH access
  ssh-add -D

  # HACK: This is a fragile attempt to add whatever key is for github.com to our ssh-agent
  grep -A 2 github.com ~/.ssh/config | grep IdentityFile | awk '{ print $2 }' | xargs -n 1 ssh-add
else
  warn "No user SSH config, skipping setting up SSH access"
fi
