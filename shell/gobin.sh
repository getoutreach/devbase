#!/usr/bin/env bash
#
# Run a golang binary using gobin

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
GOBIN_VERSION=1.7.0
GOOS=$(go env GOOS)
GOARCH=$(go env GOARCH)

# shellcheck source=./lib/bootstrap.sh
source "$DIR/lib/bootstrap.sh"
# shellcheck source=./lib/logging.sh
source "$DIR/lib/logging.sh"
# shellcheck source=./lib/shell.sh
source "$DIR/lib/shell.sh"

PRINT_PATH=false
if [[ $1 == "-p" ]]; then
  PRINT_PATH=true
  shift
fi

if [[ -z $1 ]] || [[ $1 =~ ^(--help|-h) ]]; then
  echo "Usage: $0 [-p|-h|--help] <package> [args]" >&2
  exit 1
fi

# Clone the latest version of gobin
GOBIN_PATH=$(get_cached_path "gobin" "$GOBIN_VERSION")

if [[ -z $GOBIN_PATH ]]; then
  tmp_dir=$(mktemp -d)
  # retry w/ 5s interval, 5 times
  retry 5 5 curl --location --output "$tmp_dir/gobin.tar.gz" --silent \
    "https://github.com/getoutreach/gobin/releases/download/v$GOBIN_VERSION/gobin_${GOBIN_VERSION}_${GOOS}_${GOARCH}.tar.gz"
  # shellcheck disable=SC2181 # Why: Reads better this way
  if [[ $? -ne 0 ]]; then
    echo "Error: Failed to download gobin"
    exit 1
  fi

  pushd "$tmp_dir" >/dev/null || exit 1
  tar xf "$tmp_dir/gobin.tar.gz"
  cp gobin "$GOBIN_PATH"
  popd >/dev/null || exit 1
fi

# retry w/ 5s interval, 5 times
BIN_PATH=$(retry 5 5 "$GOBIN_PATH" --skip-update -p "$1")
if [[ -z $BIN_PATH ]]; then
  echo "Error: Failed to run $1" >&2
  exit 1
fi

# Remove the module
shift

if [[ $PRINT_PATH == "true" ]]; then
  echo "$BIN_PATH"
  exit
fi

exec "$BIN_PATH" "$@"
