#!/usr/bin/env bash

# This file facilitates the release process for repositories owned by the DT.
set -eo pipefail
DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"

# shellcheck source=./../../lib/yaml.sh
source "$DIR/../../lib/yaml.sh"
# shellcheck source=./../../lib/bootstrap.sh
source "$DIR/../../lib/bootstrap.sh"

# DRYRUN is a flag that can be passed to this script to prevent it from
# actually creating a release in Github. Defaults to false and is
# configurable through the --dry-run CLI flag.
DRYRUN=false

# INCLUDE_SCRIPT is a script that is called after a Github release has
# been created. This is primarily meant to be used with creating other
# artifacts post-release.
#
# This is also run if there is no .gorereleaser.yml file in the
# repository but all other conditions are satisfied. This allows those
# repositories to do their own logic for releasing an unstable release.
INCLUDE_SCRIPT="$(get_repo_directory)/scripts/unstable-release.include.sh"

# run_unstable_include runs the INCLUDE_SCRIPT if it exists.
run_unstable_include() {
  if [[ ! -e $INCLUDE_SCRIPT ]]; then
    return 0
  fi

  # Allow users to add custom steps at the end of an unstable release
  # being created (e.g., publish artifacts).
  echo "Calling $(basename "$INCLUDE_SCRIPT")"
  export DRYRUN
  exec "$INCLUDE_SCRIPT"
}

# Set the DRYRUN flag if --dry-run is passed.
if [[ $1 == "--dry-run" ]]; then
  DRYRUN=true
fi

if [[ ! -e "$(get_repo_directory)/.goreleaser.yml" ]]; then
  echo "No .goreleaser.yml, skipping creating unstable release"

  # Run the unstable include script if it exists.
  run_unstable_include
  exit 0
fi

# If we're in dry-run mode, skip creating the release.
if [[ $DRYRUN == "true" ]]; then
  echo "this is dryrun"
  exit 0
fi

# check commit message on main branch
COMMIT_MESSAGE=$(git log --format=%B -n 1)
echo "getting commit message: $COMMIT_MESSAGE"
if [[ $COMMIT_MESSAGE == "chore: Release" ]]; then
  # run prerealse for release commit
  echo "Running RC release"
  # publish release by semantic-release

  # Read the GH_TOKEN from the file
  GH_TOKEN="$(cat "$HOME/.outreach/github.token")"
  if [[ -z $GH_TOKEN ]]; then
    echo "Failed to read Github personal access token" >&2
  fi
  # Unset NPM_TOKEN to force it to use the configured ~/.npmrc
  NPM_TOKEN='' GH_TOKEN=$GH_TOKEN \
  yarn --frozen-lockfile semantic-release
  exit
fi

# publish unstable release
app_version="v0.0.0-unstable+$(git rev-parse HEAD)"
echo "Creating unstable release ($app_version)"

make release APP_VERSION="$app_version"
# delete unstable release and unstable tag if it exists
gh release delete unstable -y || true
git tag --delete unstable || true
git push --delete origin unstable || true
# create release and upload assets to it
gh release create unstable --prerelease=true --generate-notes ./dist/*.tar.gz ./dist/checksums.txt

run_unstable_include
