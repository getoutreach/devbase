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

if [[ $? -ne 0 ]]; then
  echo "Non-zero exit code returned from curl to codecov to get repository information: $info" >&2
  exit 1
fi

# Check to see if the repository is private and not already active. In this case we'll
# need to manually activate the repository with another call to the codecov API.
# The truth table for this is the following, with P referring to .repo.prviate and A
# referring to .repo.active:
#   P | A || P and (not A)
#   ----------------------
#   0 | 0 ||     0
#   1 | 0 ||     1
#   0 | 1 ||     0
#   1 | 1 ||     0
if [[ $(jq '.repo.private and (.repo.active | not)' <"$info") == "true" ]]; then
  echo "Attempting to activate private repository."
  curl -X POST "https://codecov.io/api/pub/gh/$owner/$repo/settings" -d 'action=activate' -H "Authorization: $CODECOV_API_KEY"

  # Re-do the request to get information, upload token should exist now.
  info="$(curl -X GET "https://codecov.io/api/gh/$owner/$repo" -H "Authorization: $CODECOV_API_KEY")"

  if [[ $? -ne 0 ]]; then
    echo "Non-zero exit code returned from curl to codecov to get repository information: $info" >&2
    exit 1
  fi
fi

upload_token="$(jq -r '.repo.upload_token' <"$info")"
if [[ $upload_token == "null" ]]; then
  upload_token=""
fi

echo "$upload_token"
