#!/usr/bin/env bash
# yet another bash script to publish ruby gems
set -e

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)/.."
SCRIPTS_DIR="$DIR/../shell"
LIB_DIR="$SCRIPTS_DIR/lib"

appName="bootstraptestservice"
rubyVersion="2.6"
subDir="api/clients/ruby"
versionFile="$DIR/../../$subDir/lib/${appName}_client/version.rb"

newVersion="$1"

# shellcheck source=../lib/logging.sh
source "$LIB_DIR/logging.sh"

if [[ -z $newVersion ]]; then
  error "Expected one argument, new version, but it was not provided"
  exit 1
fi

# setup docker authentication
# shellcheck source=../lib/docker-authn.sh
source "$LIB_DIR/docker-authn.sh"

echo "setting package version to $newVersion" >&2
sed -i.bak "/VERSION /s/=.*/= \"$newVersion\"/" "$versionFile" && rm "$versionFile.bak"

# shellcheck disable=SC2001
package="$(sed 's/-rc/\.pre\.rc/' <<<"pkg/${appName}_client-$newVersion.gem")"
projectDir="$appName"
if [[ -n $CIRCLECI ]]; then
  # Note: We should fix this inconsistency eventually
  projectDir="project"
fi
prefix="/src/$projectDir/api/clients/ruby"

echo "building ruby package" >&2
mkdir -p "./pkg"
"$SCRIPTS_DIR/run-docker-container.sh" "$DIR/../../..":/src "$prefix/pkg:./" \
  -w "$prefix" gcr.io/outreach-docker/ruby:"$rubyVersion" bash -c "bundle install; bundle exec rake build"
if [[ ! -e $package ]]; then
  error "failed to find built package ($package)"
  exit 1
fi
