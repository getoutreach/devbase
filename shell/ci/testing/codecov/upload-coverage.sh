#!/usr/bin/env bash
# Uploads code coverage to codecov.io
#
# Note: This is not meant to be called outside of coverage.sh
set -e

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"

# Ensure that we have a codecov API key.
if [[ -z $CODECOV_API_KEY ]]; then
  echo "CODECOV_API_KEY environment variable is empty." >&2
  exit 1
fi

coverage_file="$1"
coverage_group="$2"

origin="$(git remote get-url origin)"
owner="$(sed -E 's/.*github.com[:\/]([^\/]+)\/([^\/]+).*/\1/' <<<"$origin")"
repo="$(basename "$(sed -E 's/.*github.com[:\/]([^\/]+)\/([^\/]+).*/\2/' <<<"$origin")" .git)"
if [[ -z $owner || -z $repo ]]; then
  echo "Could not determine owner and repo." >&2
  exit 1
fi

upload_token="$("$DIR"/get-upload-token.sh "$owner" "$repo")"
if [[ -z $upload_token ]]; then
  echo "Failed to get coverage upload token" >&2
  exit 1
fi

# Default arguments is the upload token and the coverage file
args=("-t" "$upload_token" "-f" "$coverage_file")

# If we have a coverage group, add it to the arguments
if [[ -n $coverage_group ]]; then
  args+=("-F" "$coverage_group")
fi

# If the codecov CLI is not found, download it into a temporary directory.
codecovPath="$(command -v codecov || true)"
if [[ ! -e $codecovPath ]] || [[ ! -x $codecovPath ]]; then
  tempDir="$(mktemp -d)"
  trap 'rm -rf $tempDir' EXIT

  os="linux"
  if [[ "$(uname)" == "Darwin" ]]; then
    os="macos"
  fi

  curl -sS -o "$tempDir/codecov" "https://uploader.codecov.io/latest/$os/codecov"
  chmod +x "$tempDir/codecov"
  codecovPath="$tempDir/codecov"
fi

exec "$codecovPath" "${args[@]}"
