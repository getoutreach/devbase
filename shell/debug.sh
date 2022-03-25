#!/bin/bash
set -e

SCRIPTS_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"

# shellcheck source=./lib/bootstrap.sh
source "$SCRIPTS_DIR/lib/bootstrap.sh"

export GOPROXY=https://proxy.golang.org
export GOPRIVATE="github.com/getoutreach/*"
export CGO_ENABLED=1

set -ex

if [[ -n "${DLV_PORT}" ]]; then
  exec "$SCRIPTS_DIR/gobin.sh" github.com/go-delve/delve/cmd/dlv@v"$(get_application_version "delve")" debug "$(get_repo_directory)/cmd/$(get_app_name)" --headless --listen=":${DLV_PORT}"
else
  exec "$SCRIPTS_DIR/gobin.sh" github.com/go-delve/delve/cmd/dlv@v"$(get_application_version "delve")" debug --build-flags="-tags=or_dev" "$(get_repo_directory)/cmd/$(get_app_name)"
fi
