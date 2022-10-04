#!/usr/bin/env bash
# Creates an upload key for Codecov for a given repository and
# stores it in CircleCI if running on CircleCI.
set -e

if [[ -n $CODECOV_UPLOAD_TOKEN ]]; then
  echo "Using CODECOV_UPLOAD_TOKEN environment variable." >&2
  echo -n "$CODECOV_UPLOAD_TOKEN"
  exit 0
fi

if [[ -z $CODECOV_API_KEY ]]; then
  echo "CODECOV_API_KEY environment variable is empty." >&2
  exit 1
fi

if [[ $# -ne 2 ]]; then
  echo "Usage: $(basename "$0") [owner] [repo]" >&2
  exit 1
fi

owner="$1"
repo="$2"

info="$(curl -s -o - -X GET -H "Authorization: $CODECOV_API_KEY" "https://codecov.io/api/gh/$owner/$repo")"
if [[ -z $info ]]; then
  echo "Failed to get repository information from Codecov." >&2
  exit 1
fi
active="$(jq -r '.repo.active' <<<"$info")"
private="$(jq -r '.repo.private' <<<"$info")"

# Check to see if the repository is not already active, and if it is private.
# If it's not private, we don't need to activate it.
if [[ $active == "false" ]] && [[ $private == "true" ]]; then
  echo "Activating repository $owner/$repo in codecov" >&2
  curl -X POST -H "Authorization: $CODECOV_API_KEY" -o - \
    "https://codecov.io/api/pub/gh/$owner/$repo/settings" -d 'action=activate' >&2
fi

upload_token="$(jq -r '.repo.upload_token' <<<"$info")"
if [[ $upload_token == "null" ]]; then
  upload_token=""
fi

# If we have a CirleCI API token, attempt to upload the upload token
# to CircleCI
if [[ $CIRCLECI == "true" ]] && [[ -n $CIRCLE_API_TOKEN ]]; then
  curl -s -X POST \
    -H "Content-Type: application/json" -H "Circle-Token: $CIRCLE_API_TOKEN" -H "Accept: application/json" \
    "https://circleci.com/api/v2/project/github/$owner/$repo/envvar" \
    -d "{\"name\":\"CODECOV_UPLOAD_TOKEN\",\"value\":\"$upload_token\"}" || true
fi

echo -n "$upload_token"
