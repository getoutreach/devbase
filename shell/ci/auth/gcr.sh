#!/usr/bin/env bash
# Configures CircleCI docker authentication for Google Cloud Registry (GCR).
set -e

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
LIB_DIR="${DIR}/../../lib"
AUTHN_DIR="${LIB_DIR}/docker/authn"

# shellcheck source=../../lib/logging.sh
source "${LIB_DIR}/logging.sh"

# shellcheck source=../../lib/docker/authn/gcr.sh
source "${AUTHN_DIR}/gcr.sh"

if [[ -z $GCLOUD_SERVICE_ACCOUNT ]]; then
  warn "Skipped: GCLOUD_SERVICE_ACCOUNT is not set."
  exit 0
fi

gcr_auth "$GCLOUD_SERVICE_ACCOUNT"
