#!/usr/bin/env bash
# Read local secret material written by devconfig.sh.

LIB_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"

# shellcheck source=./bootstrap.sh
source "$LIB_DIR/bootstrap.sh"

# Primary secrets location. Overridable for tests.
OUTREACH_SECRETS_DIR="${OUTREACH_SECRETS_DIR:-/run/secrets/outreach.io}"

# Fallback secrets base, per app. Overridable for tests so $HOME stays intact for mise shims.
OUTREACH_LOCAL_SECRETS_DIR="${OUTREACH_LOCAL_SECRETS_DIR:-$HOME/.outreach}"

# read_local_secret reads a secret from the well-known local paths used by
# devconfig.sh: $OUTREACH_SECRETS_DIR/<path>, falling back to
# $OUTREACH_LOCAL_SECRETS_DIR/<appName>/<path>. Prints nothing and returns
# non-zero if not found in either location. appName defaults to
# get_app_name, but can be passed explicitly to avoid re-invoking it when
# reading multiple secrets in the same script.
read_local_secret() {
  local path="$1"
  local appName="${2:-$(get_app_name)}"

  local candidates=(
    "$OUTREACH_SECRETS_DIR/$path"
    "$OUTREACH_LOCAL_SECRETS_DIR/$appName/$path"
  )
  for candidate in "${candidates[@]}"; do
    if [[ -e $candidate ]]; then
      cat "$candidate"
      return $?
    fi
  done
  return 1
}
