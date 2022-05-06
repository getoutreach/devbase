#!/usr/bin/env bash
# This script enables us to run different bins with one Air configuration
set -e
DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
# shellcheck source=./lib/bootstrap.sh
source "$DIR/lib/bootstrap.sh"

# APPNAME is the application's name.
APPNAME="$(get_app_name)"

# DEV_CONTAINER_EXECUTABLE is the executable to run in the dev container.
DEV_CONTAINER_EXECUTABLE="${DEV_CONTAINER_EXECUTABLE:-$APPNAME}"

echo "Running $DEV_CONTAINER_EXECUTABLE"
exec "$(get_repo_directory)/bin/$DEV_CONTAINER_EXECUTABLE" "$@"
