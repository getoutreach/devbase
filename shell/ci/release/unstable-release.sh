#!/usr/bin/env bash
# This file contains the logic for releasing
# unstable code on CLI containing repositories
# that have also opted to enablePrereleases _and_
# not release from the default branch (e.g. main).
set -e
DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"

# shellcheck source=./../../lib/bootstrap.sh
source "$DIR/../../lib/bootstrap.sh"

dryRun=false
if [[ $1 == "--dry-run" ]]; then
  dryRun=true
fi

# If we don't have any commands, skip this
commandsLen=$(yq -r '.arguments.commands | length' <"$(get_service_yaml)")
if [[ $commandsLen -eq 0 ]]; then
  exit 0
fi

# If we don't have pre-releasing enabled, skip this.
if [[ "$(yq -r ".arguments.releaseOptions.enablePrereleases" 2>/dev/null <"$(get_service_yaml)")" != "true" ]]; then
  exit 0
fi

# If our prereleasesBranch is empty, or equal to the default branch
# skip this.
prereleasesBranch="$(yq -r '.arguments.releaseOptions.prereleasesBranch' <"$(get_service_yaml)")"
defaultBranch="$(git rev-parse --abbrev-ref origin/HEAD | sed 's/^origin\///')"
if [[ -z $prereleasesBranch ]] || [[ $prereleasesBranch == "$defaultBranch" ]]; then
  exit 0
fi

app_version="unstable-$(git rev-parse --short HEAD)"
echo "Creating unstable release ($app_version)"

make release APP_VERSION="$app_version"

# If we're not on the prereleases branch or dryRun, skip uploading.
currentBranch="$(git rev-parse --abbrev-ref HEAD)"
if [[ $currentBranch != "$prereleasesBranch" ]] || [[ $dryRun == "true" ]]; then
  exit 0
fi

# delete unstable release/tag if it exists
gh release delete unstable || true

# create unstable release and upload assets to it
gh release create unstable --generate-notes ./dist/*.tar.gz ./dist/checksums.txt