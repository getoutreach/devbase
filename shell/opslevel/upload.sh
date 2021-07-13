#!/usr/bin/env bash
#
# This script uploads various build stats to OpsLevel.
# At first, our plan is to upload Prisma Cloud image scan results
# (vulnerabilities and compliance issues), more to come later.
#

# We only upload to opslevel on tagged releases.
if [[ -z $CIRCLE_TAG ]]; then
  echo "Skip OpsLevel upload since release tag is missing"
  exit 0
fi
DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"

# Preload libs that all others opslevel/* scripts rely on.

# shellcheck source=../lib/artifacts.sh
source "$DIR/../lib/artifacts.sh"
# shellcheck source=../lib/opslevel.sh
source "$DIR/../lib/opslevel.sh"

# Download Artifacts JSON once
ARTIFACTS_JSON_FILE=/tmp/artifacts.json
download_artifacts_json "${ARTIFACTS_JSON_FILE}"

# Upload scan results to Prisma Cloud
# shellcheck source=./upload_prismaci.sh
source "$DIR/upload_prismaci.sh" "${ARTIFACTS_JSON_FILE}"
