#!/usr/bin/env bash
# Opens a pull request that upgrades the Stencil modules of the
# repository, or adds the upgrade to the open pull request from an
# earlier run. See stencil_upgrade_run in lib/stencil_upgrade.sh for the
# environment variables that it reads.
set -eo pipefail
DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"

# shellcheck source=./../lib/bootstrap.sh
source "$DIR/../lib/bootstrap.sh"
# shellcheck source=./../lib/github.sh
source "$DIR/../lib/github.sh"
# shellcheck source=./../lib/stencil_upgrade.sh
source "$DIR/../lib/stencil_upgrade.sh"

# shellcheck disable=SC2119
# Why: no extra args needed to pass to ghaccesstoken in this case.
bootstrap_github_token

stencil_upgrade_run
