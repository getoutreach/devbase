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

# If we don't have a .goreleaser file, skip this.
# TODO(jaredallard)[DT-2796]: This enables plugins to release from main.
if [[ ! -e "$(get_repo_directory)/.goreleaser.yml" ]]; then
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

app_version="v0.0.0-unstable+$(git rev-parse HEAD)"
echo "Creating unstable release ($app_version)"

make release APP_VERSION="$app_version"

if [[ $dryRun == "true" ]]; then
  exit 0
fi

# delete unstable release+tag if it exists
gh release delete unstable -y || true
git tag --delete unstable || true
git push --delete origin unstable || true

# create unstable release and upload assets to it
gh release create unstable --prerelease --generate-notes ./dist/*.tar.gz ./dist/checksums.txt
