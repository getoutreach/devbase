#!/usr/bin/env bash
# Release some code
DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
LIB_DIR="${DIR}/../../lib"
nodeClientDir="api/clients/node"

# shellcheck source=../../lib/logging.sh
source "${LIB_DIR}/logging.sh"

# Unset NPM_TOKEN to force it to use the configured ~/.npmrc
NPM_TOKEN='' GH_TOKEN=$GITHUB_TOKEN \
  yarn --frozen-lockfile semantic-release

# Special logic to publish a node client to github packages while
# we're dual writing. This will be removed soonish.
if [[ -e $nodeClientDir ]]; then
  info "Publishing node client to Github Packages"

  info_sub "pointing package.json to Github Packages"
  pjson="$nodeClientDir/package.json"
  originalName="$(jq -r '.name' "$pjson")"
  newName="${originalName//@outreach/@getoutreach}"

  newpjson="$(jq ". + {\"name\":\"$newName\",\"publishConfig\":{\"registry\":\"https://npm.pkg.github.com/\"}}" "$pjson")"
  echo "$newpjson" >"$pjson"

  pushd "$nodeClientDir" >/dev/null || exit 1
  info_sub "pushing to github packages"
  npm publish
  popd >/dev/null || exit 1
fi
