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

appName="$(get_app_name)"
clientDir="$(get_repo_directory)/api/clients/ruby"
versionFile="$clientDir/lib/${appName}_client/version.rb"

newVersion="$1"
if [[ -z $newVersion ]]; then
  echo "Error: Must pass in version" >&2
  exit 1
fi

echo "Setting package version to $newVersion" >&2
sed -i.bak "/VERSION /s/=.*/= \"$newVersion\"/" "$versionFile" && rm "$versionFile.bak"

echo "Building ruby package"
pushd "$clientDir" >/dev/null || exit 1
bundle install
bundle exec rake build
popd >/dev/null || exit 1
