#!/usr/bin/env bash
# This script enables us to run different bins with one Air configuration
set -e

if [[ $SKIP_STARTING_APP == "true" ]]; then
  echo "Application was built into bin folder, use 'devenv apps run -b .' and to start devspace pod with binary sync (if you haven't already)"
  exit
fi

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
# shellcheck source=./lib/bootstrap.sh
source "$DIR/lib/bootstrap.sh"

# APPNAME is the application's name.
APPNAME="$(get_app_name)"

# DEV_CONTAINER_EXECUTABLE is the executable to run in the dev container.
DEV_CONTAINER_EXECUTABLE="${DEV_CONTAINER_EXECUTABLE:-$APPNAME}"

if [[ -z $KUBERNETES_SERVICE_HOST ]]; then
  exec "$(get_repo_directory)/bin/$DEV_CONTAINER_EXECUTABLE" "$@"
  exit
fi

if [[ -z $DEVBOX_LOGFMT ]] && [[ -z $LOGFMT_FORMAT ]] && [[ -z $LOGFMT_FILTER ]]; then
  exec "$(get_repo_directory)/bin/$DEV_CONTAINER_EXECUTABLE" "$@" | tee -ai "${DEV_CONTAINER_LOGFILE:-/tmp/app.log}"
  exit
fi

logfmt=(
  $"$DIR/gobin.sh"

  "github.com/getoutreach/logfmt/cmd/logfmt@$(get_tool_version "getoutreach/logfmt")"
)

if [[ -n $LOGFMT_FORMAT ]]; then
  logfmt+=(--format "$LOGFMT_FORMAT")
fi

if [[ -n $LOGFMT_FILTER ]]; then
  logfmt+=(--filter "$LOGFMT_FILTER")
fi

exec "$(get_repo_directory)/bin/$DEV_CONTAINER_EXECUTABLE" "$@" |
  tee -ai "${DEV_CONTAINER_LOGFILE:-/tmp/app.log}" |
  "${logfmt[@]}"
