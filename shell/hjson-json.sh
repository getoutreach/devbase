#!/usr/bin/env bash
DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
GOBIN="$DIR/gobin.sh"

# shellcheck source=./lib/bootstrap.sh
source "$DIR/lib/bootstrap.sh"

HJSON_CLI=$("$GOBIN" -p github.com/hjson/hjson-go/hjson-cli@ca8d4fec02fe4da51776c0c1f4faac480a61eaa9)

EXISTING_VERSION="0.0.1"
if [[ -e $2 ]]; then
  EXISTING_VERSION=$(jq -r .version "$2")
fi

CONVERTED_JSON=$("$HJSON_CLI" -c "$1")

WARNING_COMMENT="{\"//\": \"DO NOT EDIT, EDIT $1\"}"
VERSION="{\"version\": \"$EXISTING_VERSION\"}"

exec jq "$WARNING_COMMENT + . + $VERSION" <<<"$CONVERTED_JSON" >"$2"
