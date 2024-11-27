#!/usr/bin/env bash
#
# GCR authentication. Assumes that logging.sh is sourced.
#

set -eo pipefail

gcr_auth() {
  gcloudServiceAccount="$1"
  if [[ -z $gcloudServiceAccount ]]; then
    warn "Skipped: gcloudServiceAccount is not set."
    return
  fi

  docker login \
    -u _json_key \
    --password-stdin \
    https://gcr.io <<<"${gcloudServiceAccount}"
}
