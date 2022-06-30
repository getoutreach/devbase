#!/usr/bin/env bash

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"

if [[ -z $CODECOV_API_KEY ]]; then
  echo "CODECOV_API_KEY environment variable is empty." >&2
  exit 1
fi

if [[ $CIRCLECI == "true" ]]; then
  owner="${CIRCLE_PROJECT_USERNAME}"
  repo="${CIRCLE_PR_REPONAME:-$CIRCLE_PROJECT_REPONAME}"
else
  if [[ $# -lt 2 ]]; then
    echo "When running outside of CIRCLECI the first two positional arguments passed to this script need to be owner and repository." >&2
    exit 1
  fi

  owner="$1"
  shift
  repo="$1"
  shift
fi

upload_token="$("$DIR"/get-upload-token.sh "$owner" "$repo")"

# shellcheck disable=SC2181
if [[ $? -ne 0 ]]; then
  echo "Failed to get upload token from codecov API." >&2
  exit 1
fi

args=("-t" "$upload_token")
if [[ $# -gt 0 ]]; then
  args=("${args[@]}" "$@")
fi

# Install codecov binary if it doesn't already exist in $PATH
if [[ ! -x "$(command -v codecov)" ]]; then
  os="linux"
  if [[ "$(uname)" == "Darwin" ]]; then
    os="macos"
  fi

  curl -OsS "https://uploader.codecov.io/latest/$os/codecov"
  chmod +x codecov
  sudo mv codecov /usr/local/bin
fi

if ! codecov "${args[@]}"; then
  echo "codecov upload failed." >&2
  exit 1
fi

echo "Succesfully uploaded codecov report."
exit 0
