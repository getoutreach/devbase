#!/usr/bin/env bash
# Publish a Python package to GitHub Packages using twine
set -euo pipefail

# Determine directories relative to this script.
DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)/.."
SCRIPTS_DIR="$DIR/../shell"
LIB_DIR="$SCRIPTS_DIR/lib"

# Source helper scripts (bootstrap and logging)
# shellcheck source=../lib/bootstrap.sh
source "$LIB_DIR/bootstrap.sh"
# shellcheck source=../lib/logging.sh
source "$LIB_DIR/logging.sh"

newVersion="$1"
if [[ -z $newVersion ]]; then
  error "Expected one argument: new version, but it was not provided"
  exit 1
fi

DRYRUN_MODE=true
if [[ "${DRYRUN:-}" == "false" ]]; then
  DRYRUN_MODE=false
fi

clientDir="$(get_repo_directory)/api/clients/python"
distDir="$clientDir/dist"

# Ensure the distribution directory exists and has files.
if [[ ! -d $distDir ]]; then
  error "Distribution directory '$distDir' does not exist. Please build the package first."
  exit 1
fi

if ! compgen -G "$distDir/*" > /dev/null; then
  error "No distribution files found in '$distDir'. Please build the package first."
  exit 1
fi

if [[ $DRYRUN_MODE == "true" ]]; then
  warn "Dry-run: Skipping package upload."
  exit 0
fi

pushd "$clientDir" >/dev/null || exit 1
info "Uploading Python package (version $newVersion) to GitHub Packages"

# Upload the package using twine.
# This command assumes that you have a properly configured .pypirc file with a repository named "github".
twine upload --repository github dist/*

popd >/dev/null || exit 1
