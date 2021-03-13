#!/usr/bin/env bash
# yet another bash script to publish ruby gems
set -e

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)/.."
SCRIPTS_DIR="$CIRCLE_DIR/../scripts"
LIB_DIR="$SCRIPTS_DIR/lib"

appName="bootstraptestservice"

newVersion="$1"

# shellcheck source=../lib/logging.sh
source "$LIB_DIR/logging.sh"

DRYRUN_MODE=true
if [[ $DRYRUN == "false" ]] || [[ -z $DRYRUN ]]; then
  info "not running in dry run mode" >&2
  DRYRUN_MODE=false
fi

# Ensure we have the token, unless we're in dryrun mode
if [[ -z $PACKAGECLOUD_TOKEN ]] && [[ -z $DRYRUN ]]; then
  # Why: We don't want $PACKAGECLOUD_TOKEN to be expanded here.
  # shellcheck disable=SC2016
  error 'No packagecloud ($PACKAGECLOUD_TOKEN) token provided'
  exit 1
fi

if [[ -z $newVersion ]]; then
  error "Expected one argument, new version, but it was not provided"
  exit 1
fi

# setup docker authentication
# shellcheck source=../lib/docker-authn.sh
source "$DIR/docker-authn.sh"

# shellcheck disable=SC2001
package="$(sed 's/-rc/\.pre\.rc/' <<<"pkg/${appName}_client-$newVersion.gem")"

if [[ $DRYRUN_MODE == "true" ]]; then
  warn "skipping publish of ruby package, in dry-run mode"
  info "Would've ran: package_cloud push outreach/rubygems \"$package\""
  exit 0
fi

info "pushing to packagecloud" >&2

"$CIRCLE_DIR/run-docker-container.sh" "$DIR/../../pkg":/src -- \
  -w "/src" -e "PACKAGECLOUD_TOKEN=$PACKAGECLOUD_TOKEN" gcr.io/outreach-docker/package-cloud \
  package_cloud push outreach/rubygems "$package"
