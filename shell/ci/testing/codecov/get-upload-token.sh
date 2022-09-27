#!/usr/bin/env bash

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

info="$(curl -X GET "https://codecov.io/api/gh/$owner/$repo" -H "Authorization: $CODECOV_API_KEY")"

# shellcheck disable=SC2181
if [[ $? -ne 0 ]]; then
  echo "Non-zero exit code returned from curl to codecov to get repository information: $info" >&2
  exit 1
fi

# Check to see if the repository is not already active.
if [[ $(jq '.repo.active | not' <<<"$info") == "true" ]]; then
  echo "Attempting to activate repository." >&2
  curl -X POST "https://codecov.io/api/pub/gh/$owner/$repo/settings" -d 'action=activate' -H "Authorization: $CODECOV_API_KEY"
fi

upload_token="$(jq -r '.repo.upload_token' <<<"$info")"
if [[ $upload_token == "null" ]]; then
  upload_token=""
fi

echo -n "$upload_token"
