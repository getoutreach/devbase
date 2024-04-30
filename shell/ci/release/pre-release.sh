#!/usr/bin/env bash

# This file contains the logic for releasing unstable code on CLI
# containing repositories that have also opted to enablePrereleases
# _and_ not release from the default branch (e.g. main).
set -eo pipefail
DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"

# shellcheck source=./../../lib/yaml.sh
source "$DIR/../../lib/yaml.sh"
# shellcheck source=./../../lib/bootstrap.sh
source "$DIR/../../lib/bootstrap.sh"

# CIRCLE_BRANCH is the current branch we're on. If we're unable to
# determine the current branch (e.g., not running in CI) we fallback to
# attempting to parse the current branch from git.
CIRCLE_BRANCH="${CIRCLE_BRANCH:-$(git rev-parse --abbrev-ref HEAD)}"

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

# If we don't have pre-releasing enabled, skip this.
if [[ "$(yaml_get_field ".arguments.releaseOptions.enablePrereleases" "$(get_service_yaml)")" != "true" ]]; then
  echo "releaseOptions.enablePrereleases is not true, skipping unstable release"
  exit 0
fi

# Ensure we are on the prerelease branch. The default is main branch.
prereleasesBranch="$(yaml_get_field '.arguments.releaseOptions.prereleasesBranch' "$(get_service_yaml)")"

# If we're not on the prerelease branch, skip. This is to prevent
# accidentally releasing from a branch that isn't mean to create
# unstable releases that happened to fail releasing for whatever reason.
#
# Special case, skip this check if we're doing a dry-run since we will
# short circuit before we actually create a release.
if [[ $CIRCLE_BRANCH != "$prereleasesBranch" ]] && [[ $DRYRUN == "false" ]]; then
  echo "\$CIRCLE_BRANCH ($CIRCLE_BRANCH) != \$prereleaseBranch ($prereleasesBranch), skipping prerelease"
  exit 0
fi

# If we're in dry-run mode, skip creating the release.
if [[ $DRYRUN == "true" ]]; then
  exit 0
fi

# Check commit message on current branch
# Prerelease to rc if the latest commit is chore: Release.
COMMIT_MESSAGE=$(git log --format=%B -n 1)
if [[ $COMMIT_MESSAGE =~ "chore: Release" ]]; then
  # Pre-release to rc
  echo "Creating prerelease to rc channel"

  # Retrieve the GH_TOKEN
  GH_TOKEN=$(gh auth token)
  if [[ -z $GH_TOKEN ]]; then
    echo "Failed to read Github personal access token" >&2
  fi
  # Unset NPM_TOKEN to force it to use the configured ~/.npmrc
  NPM_TOKEN='' GH_TOKEN=$GH_TOKEN \
    yarn --frozen-lockfile semantic-release
  exit 0
fi

# If there's no .goreleaser.yml file, skip the unstable release process.
# Otherwise, the 'make release' step would fail.
#
# IDEA(jaredallard): We should support a more customizable release
# process for things that don't use goreleaser.
if [[ ! -e "$(get_repo_directory)/.goreleaser.yml" ]]; then
  echo "No .goreleaser.yml, skipping creating unstable release"

  # Run the unstable include script if it exists.
  run_unstable_include
  exit 0
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
