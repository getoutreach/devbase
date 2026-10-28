#!/usr/bin/env bash
#
# vault: a minimal replacement for the `vault` CLI, backed by `devbase
# vault` -- see shell/cli.sh for how the devbase CLI itself is resolved
# (a tagged release binary, or a build from source).
#
# It only implements what devbase's own scripts use. Any other command or
# flag fails (see cmd/devbase/vault.go), so CI does not need the `vault` CLI.
#
# A tagged devbase release only needs curl. An unstable or local devbase
# version is built from source, so it needs Go; unlike shell/yq.sh there is
# no fallback, as no other tool replaces `devbase vault`.
#

set -euo pipefail

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"

# shellcheck source=./cli.sh
source "${DIR}/cli.sh"

devbase_cli vault "$@"
