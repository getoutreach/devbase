#!/usr/bin/env bash
# Release some code
set -e

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
LIB_DIR="${DIR}/../../lib"
nodeClientDir="api/clients/node"

# Read the GH_TOKEN from the file
GH_TOKEN="$(cat "$HOME/.outreach/github.token")"
if [[ -z $GH_TOKEN ]]; then
  echo "Failed to read Github personal access token" >&2
fi

send_failure_notification() {
  if [[ -z $RELEASE_FAILURE_SLACK_CHANNEL ]]; then
    echo "Failed to release"
    exit 1
  fi

  curl -X POST "$RELEASE_FAILURE_WEBHOOK" \
    -H 'Content-Type: application/json' \
    -d '{"slackChannel": "'"$RELEASE_FAILURE_SLACK_CHANNEL"'", "buildURL": "'"$CIRCLE_BUILD_URL"'", "repoName": "'"$CIRCLE_PROJECT_REPONAME"'"}'
  exit 1
}

# shellcheck source=../../lib/logging.sh
source "${LIB_DIR}/logging.sh"

# shellcheck source=../../lib/bootstrap.sh
source "${LIB_DIR}/bootstrap.sh"

# shellcheck source=../../lib/box.sh
source "${LIB_DIR}/box.sh"

OPSLEVEL_ENABLED="$(get_box_field '.ci.opslevelEnabled')"
if [[ $OPSLEVEL_ENABLED == "true" && "$(is_service)" == "false" ]]; then
  echo "checking opslevel"
  make checkopslevel || send_failure_notification
fi

ORIGINAL_VERSION=$(git describe --match 'v[0-9]*' --tags --always HEAD)

# Unset NPM_TOKEN to force it to use the configured ~/.npmrc
NPM_TOKEN='' GH_TOKEN=$GH_TOKEN \
  yarn --frozen-lockfile semantic-release || send_failure_notification

NEW_VERSION=$(git describe --match 'v[0-9]*' --tags --always HEAD)

# Determine if we updated by checking the original version from git
# vs the new version (potentially) after we ran semantic-release.
UPDATED=false
if [[ $ORIGINAL_VERSION != "$NEW_VERSION" ]]; then
  UPDATED=true
fi

# If we didn't update, assume we're on a prerelease branch
# and run the unstable-release code.
if [[ $UPDATED == "false" ]]; then
  "$DIR/unstable-release.sh"
elif [[ $UPDATED == "true" ]]; then
  # Special logic to publish a node client to github packages while
  # we're dual writing. This will be removed soonish.
  if [[ -e $nodeClientDir && "$(is_service)" == "true" ]]; then
    info "Publishing node client to Github Packages"

    info_sub "pointing package.json to Github Packages"
    pjson="$nodeClientDir/package.json"
    originalName="$(jq -r '.name' "$pjson")"
    newName="${originalName//@outreach/@getoutreach}"

    newpjson="$(jq ". + {\"name\":\"$newName\",\"publishConfig\":{\"registry\":\"https://npm.pkg.github.com/\"}}" "$pjson")"
    echo "$newpjson" >"$pjson"

    pushd "$nodeClientDir" >/dev/null || exit 1
    info_sub "pushing to github packages"
    npm publish || send_failure_notification
    popd >/dev/null || exit 1
  fi
fi
