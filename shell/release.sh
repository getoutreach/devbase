#!/usr/bin/env bash
#
# Creates a release for the repository using devbase.

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
DEVBASE_LIB_DIR="$DIR/lib"

# shellcheck source=./lib/mise/stub.sh
source "$DEVBASE_LIB_DIR/mise/stub.sh"

appVersion="${APP_VERSION:-$(get_app_version)}"
# Create a tag for our version
git tag -d "$appVersion" >&2 >/dev/null || true
git tag "$appVersion" >&2
GORELEASER_CURRENT_TAG="$appVersion" mise_exec_tool goreleaser release \
  --skip-announce --skip-publish --skip-validate --clean
# Delete the tag once we are done.
git tag -d "$appVersion" >&2
