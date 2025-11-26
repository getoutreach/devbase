#!/usr/bin/env bash
# Trigger a private pkg.go.dev instance

set -e

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
LIB_DIR="${DIR}/../../lib"

# shellcheck source=../../lib/bootstrap.sh
source "${LIB_DIR}/bootstrap.sh"

# shellcheck source=../../lib/box.sh
source "${LIB_DIR}/box.sh"

# shellcheck source=../../lib/logging.sh
source "${LIB_DIR}/logging.sh"

TAG="$CIRCLE_TAG"
# Do not update engdocs unless there is a tag
if [[ -n $TAG && -f "$(get_repo_directory)/go.mod" ]]; then
  repo="$(get_box_field org)/$(get_app_name)"
  # We need to use the module path to support major versions properly.
  # Filter out module paths that aren't a part of this repo.
  MODULE_PATHS="$(go list -f '{{ .Path }}' -m | grep "$repo")"

  BASE_URL="$(get_box_field engdocs.URL)"

  info "updating engdocs"
  # There may be multiple module paths in a repo using workspaces.
  for MODULE_PATH in $MODULE_PATHS; do
    URL="$BASE_URL/fetch/$MODULE_PATH@$TAG"

    info_sub "$MODULE_PATH@$TAG"
    curl -X POST "$URL"
  done
fi

# Confluence docs get updated every run
info "publishing eligible markdown documents to confluence"
exec "$DIR/confluence-publish.sh"
