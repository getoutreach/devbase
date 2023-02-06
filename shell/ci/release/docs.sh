#!/usr/bin/env bash
# Trigger a private pkg.go.dev instance
set -e

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
LIB_DIR="${DIR}/../../lib"

# shellcheck source=../../lib/logging.sh
source "${LIB_DIR}/logging.sh"

TAG="$CIRCLE_TAG"
# Do not update engdocs unless there is a tag
if [[ -n $TAG ]]; then
  # We need to use the module path to support major versions properly
  MODULE_PATH="$(go list -f '{{ .Path }}' -m)"

  # TODO(jaredallard): Move this into box configuration?
  URL="https://engdocs.outreach.cloud/fetch/$MODULE_PATH@$TAG"

  info "updating engdocs"
  curl -X POST "$URL"
fi

# Confluence docs get updated every run
info "publishing eligible markdown documents to confluence"
exec "$DIR/confluence-publish.sh"
