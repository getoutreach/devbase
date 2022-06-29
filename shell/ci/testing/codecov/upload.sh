#!/usr/bin/env bash

args=()
if [[ -z $CODECOV_UPLOAD_TOKEN ]]; then
  args+=("-t $CODECOV_UPLOAD_TOKEN")
fi

if [[ $# -gt 0 ]]; then
  args=("${args[@]}" "$@")
fi

# Install codecov binary if it doesn't already exist in $PATH
if [[ ! -x "$(command -v codecov)" ]]; then
  os="linux"
  if [[ "$(uname)" == "Darwin" ]]; then
    os="macos"
  fi

  pushd /usr/local/bin >/dev/null 2>&1 || exit 1
  curl -Os https://uploader.codecov.io/latest/"$os"/codecov
  chmod +x codecov
  popd >/dev/null 2>&1 || exit 1
fi

if codecov "${args[@]}"; then
  echo "Succesfully uploaded codecov report."
  exit 0
fi

if [[ $CIRCLECI != "true" ]]; then
  echo "codecov uploader returned a non-zero exit code. Repository may not be activated, run ./activate-repo.sh <owner> <repo> and try again." >&2
  exit 1
fi

echo "codecov uploader returned a non-zero exit code. Attempting to activate repository based off of CIRCLECI environment variables for owner and repository name."

owner="${CIRCLE_PROJECT_USERNAME}"
repo="${CIRCLE_PR_REPONAME:-$CIRCLE_PROJECT_REPONAME}"

if ! ./activate-repo.sh "$owner" "$repo"; then
  echo "Activating the repository failed." >&2
  exit 1
fi

if codecov "${args[@]}"; then
  echo "Succesfully uploaded codecov report."
  exit 0
fi

echo "Unable to upload codecov report." >&2
exit 1
