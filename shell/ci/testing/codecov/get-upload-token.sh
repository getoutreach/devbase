#!/usr/bin/env bash
# Creates an upload key for Codecov for a given repository and
# stores it in CircleCI if running on CircleCI.
set -e

if [[ -n $CODECOV_UPLOAD_TOKEN ]]; then
  echo "Using CODECOV_UPLOAD_TOKEN environment variable." >&2
  echo "$CODECOV_UPLOAD_TOKEN"
  exit 0
fi

if [[ -z $CODECOV_API_KEY ]]; then
  echo "CODECOV_API_KEY environment variable is empty." >&2
  exit 1
fi

if [[ $# -ne 2 ]]; then
  echo "Script expects the owner/organization and name of repository passed to it as parameters." >&2
  exit 1
fi

owner="$1"
repo="$2"

info="$(curl -s -X GET "https://codecov.io/api/gh/$owner/$repo" -H "Authorization: $CODECOV_API_KEY")"

# shellcheck disable=SC2181
if [[ $? -ne 0 ]]; then
  echo "Non-zero exit code returned from curl to codecov to get repository information: $info" >&2
  exit 1
fi

# Check to see if the repository is not already active.
if [[ $(jq '.repo.active | not' <<<"$info") == "true" ]]; then
  echo "Attempting to activate repository." >&2
  curl -s -X POST "https://codecov.io/api/pub/gh/$owner/$repo/settings" -d 'action=activate' -H "Authorization: $CODECOV_API_KEY"
fi

upload_token="$(jq -r '.repo.upload_token' <<<"$info")"
if [[ $upload_token == "null" ]]; then
  upload_token=""
fi

# If we have a CirleCI API token, attempt to upload the upload token
# to CircleCI
if [[ $CIRCLECI == "true" ]] && [[ -n $CIRCLE_API_TOKEN ]]; then
  curl -s -H "Content-Type: application/json" -H "Circle-Token: $CIRCLE_API_TOKEN" -H "Accept: application/json" \
    -X POST "https://circleci.com/api/v2/project/github/$owner/$repo/envvar" \
    -d "{\"name\":\"CODECOV_UPLOAD_TOKEN\",\"value\":\"$upload_token\"}" || true
fi

echo -n "$upload_token"
