#!/usr/bin/env bash
# This is a wrapper around gobin.sh to run shellcheck.
# Useful for using the correct version of shellcheck
# with your editor.

SCRIPTS_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"

# shellcheck source=./lib/bootstrap.sh
source "$SCRIPTS_DIR/lib/bootstrap.sh"
# shellcheck source=./lib/logging.sh
source "$SCRIPTS_DIR/lib/logging.sh"
# shellcheck source=./lib/shell.sh
source "$SCRIPTS_DIR/lib/shell.sh"

SHELLCHECK_VERSION="$(get_application_version "shellcheck")"
GOOS=$(go env GOOS)
ARCH=$(uname -m)

# No builds for M1 macs at the moment, so just download
# the amd64 build.
if [[ $GOOS == "darwin" ]] && [[ $ARCH == "arm64" ]]; then
  ARCH="x86_64"
fi

# Always set the correct script directory.
args=("-P" "SCRIPTDIR" "-x" "$@")

binPath=$(get_cached_binary "shellcheck" "$SHELLCHECK_VERSION")

if [[ -z $binPath ]]; then
  {
    binPath=$(cached_binary_path "shellcheck" "$SHELLCHECK_VERSION")
    tmp_dir=$(mktemp -d)

    # retry w/ 5s interval, 5 tgimes
    retry 5 5 curl --fail --location --output "$tmp_dir/shellcheck.tar.xz" --silent \
      "https://github.com/koalaman/shellcheck/releases/download/v$SHELLCHECK_VERSION/shellcheck-v$SHELLCHECK_VERSION.$GOOS.$ARCH.tar.xz"

    pushd "$tmp_dir" >/dev/null || exit 1
    tar xf shellcheck.tar.xz
    mv "shellcheck-v$SHELLCHECK_VERSION/shellcheck" "$binPath"
    chmod +x "$binPath"
    popd >/dev/null || exit 1
    rm -rf "$tmp_dir"
  } >&2
fi

exec "$binPath" "${args[@]}"
