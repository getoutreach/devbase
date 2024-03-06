#!/usr/bin/env bash
# This script is to add chore: Release comit to the default pre-release
# branch to trigger pre-release.
set -eo pipefail
DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"

# shellcheck source=./../../lib/yaml.sh
source "$DIR/../../lib/yaml.sh"
# shellcheck source=./../../lib/bootstrap.sh
source "$DIR/../../lib/bootstrap.sh"
# shellcheck source=./../../lib/box.sh
source "$DIR/../../lib/box.sh"

if [[ "$(yaml_get_field ".arguments.releaseOptions.enablePrereleases" "$(get_service_yaml)")" != "true" ]]; then
  echo "releaseOptions.enablePrereleases is not true, skipping rc release"
  exit 0
fi

# Default it to use main branch, should this be configurable?
prereleaseBranch="main"

if [[ -n "$(yaml_get_field ".arguments.releaseOptions.releaseUser" "$(get_service_yaml)")" ]]; then
  releaseUsername="$(yaml_get_field ".arguments.releaseOptions.releaseUser.name" "$(get_service_yaml)")"
  releaseUseremail="$(yaml_get_field ".arguments.releaseOptions.releaseUser.email" "$(get_service_yaml)")"
else
  releaseUsername=$(get_box_field 'ci.circleci.releaseUser.name')
  releaseUseremail=$(get_box_field 'ci.circleci.releaseUser.email')
fi

git config --global user.name "$releaseUsername"
git config --global user.email "$releaseUseremail"
git checkout $prereleaseBranch

# Dryrun the semantic-release on prereleaseBranch to check if there is changes to release.
# If not skip the release.
GH_TOKEN=$(gh auth token)
releaseOutput=$(NPM_TOKEN='' GH_TOKEN=$GH_TOKEN yarn --frozen-lockfile semantic-release -d)
echo "$releaseOutput"

if [[ $releaseOutput != *"Published release"* ]]; then
  echo "No release will be created, skipping..."
  exit 0
fi

git commit -m "chore: Release" --allow-empty
git push origin $prereleaseBranch
