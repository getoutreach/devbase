#!/usr/bin/env bash
# Builds a ruby gem
set -e

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)/.."
SCRIPTS_DIR="$DIR/../shell"
LIB_DIR="$SCRIPTS_DIR/lib"

# shellcheck source=../lib/bootstrap.sh
source "$LIB_DIR/bootstrap.sh"
# shellcheck source=../lib/logging.sh
source "$LIB_DIR/logging.sh"
# shellcheck source=../lib/sed.sh
source "$LIB_DIR/sed.sh"

appName="$(get_app_name)"
clientDir="$(get_repo_directory)/api/clients/ruby"
versionFile="$clientDir/lib/${appName}_client/version.rb"

newVersion="$1"
if [[ -z $newVersion ]]; then
  fatal "Must pass in version"
fi

info "Setting package version to $newVersion"
sed_in_place "/VERSION /s/=.*/= \"$newVersion\"/" "$versionFile"

info "Building ruby package"
pushd "$clientDir" >/dev/null || exit 1
bundle install
bundle exec rake build
popd >/dev/null || exit 1
