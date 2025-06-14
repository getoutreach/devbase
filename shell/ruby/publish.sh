#!/usr/bin/env bash
# Publish a gem
set -e

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)/.."
SCRIPTS_DIR="$DIR/../shell"
LIB_DIR="$SCRIPTS_DIR/lib"

# shellcheck source=../lib/bootstrap.sh
source "$LIB_DIR/bootstrap.sh"
# shellcheck source=../lib/box.sh
source "$LIB_DIR/box.sh"
# shellcheck source=../lib/logging.sh
source "$LIB_DIR/logging.sh"

appName="$(get_app_name)"
clientDir="$(get_repo_directory)/api/clients/ruby"

newVersion="$1"
if [[ -z $newVersion ]]; then
  fatal "Expected one argument, new version, but it was not provided"
fi

DRYRUN_MODE=true
if [[ $DRYRUN == "false" ]] || [[ -z $DRYRUN ]]; then
  DRYRUN_MODE=false
fi

# replace -rc with .pre.rc which is what rake build does
gemFile="$clientDir/pkg/${appName}_client-${newVersion//-rc/.pre.rc}.gem"
if [[ ! -e $gemFile ]]; then
  fatal ".gem file not found at '$gemFile', refusing to publish nothing"
fi

if [[ $DRYRUN_MODE == "true" ]]; then
  warn "Skipping publish, in dry-run"
  exit 0
fi

pushd "$clientDir" >/dev/null || exit 1
info "Pushing package to Github Packages"
gem push --key github \
  --host https://rubygems.pkg.github.com/"$(get_box_field org)" \
  "$gemFile"
popd >/dev/null || exit 1
