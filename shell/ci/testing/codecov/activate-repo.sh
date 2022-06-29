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

if [[ $? -ne 1 ]]; then
  echo "Non-zero exit code returned from curl to codecov to get repository information: $info" >&2
  exit 1
fi

# Check to see if the repository needs activated. The truth table for this is the
# following, with P referring to .repo.prviate and A referring to .repo.active:
#   P | A || P and (not A)
#   ----------------------
#   0 | 0 ||     0
#   1 | 0 ||     1
#   0 | 1 ||     0
#   1 | 1 ||     0
if [[ $(jq '.repo.private and (.repo.active | not)' <"$info") == "false" ]]; then
  echo "Repository either already activated or not a private repository, exiting gracefully."
  exit 0
fi

curl -X POST "https://codecov.io/api/pub/gh/$owner/$repo/settings" -d 'action=activate' -H "Authorization: $CODECOV_API_KEY"
