#!/bin/bash
set -e

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"

# shellcheck source=./lib/bootstrap.sh
source "$DIR/lib/bootstrap.sh"

export GOPROXY=https://proxy.golang.org
export GOPRIVATE="github.com/getoutreach/*"
export CGO_ENABLED=1

if [[ -z ${DLV_PORT} ]] && [[ -z $KUBERNETES_SERVICE_HOST ]]; then
  exec "$SCRIPTS_DIR/gobin.sh" github.com/go-delve/delve/cmd/dlv@v"$(get_application_version "delve")" debug --build-flags="-tags=or_dev" "$(get_repo_directory)/cmd/$(get_app_name)"
  exit
fi

delve=(
  "$DIR/gobin.sh"
  github.com/go-delve/delve/cmd/dlv@v"$(get_application_version "delve")"
  exec
  "$(get_repo_directory)/bin/${DEV_CONTAINER_EXECUTABLE:-$(get_app_name)}"
  --headless
  --listen=":${DLV_PORT}"
)

if [[ -z $DEVBOX_LOGFMT ]] && [[ -z $LOGFMT_FORMAT ]] && [[ -z $LOGFMT_FILTER ]]; then
  exec "${delve[@]}" |
    tee -ai "${DEV_CONTAINER_LOGFILE:-/tmp/app.log}"

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

exec "${delve[@]}" |
  tee -ai "${DEV_CONTAINER_LOGFILE:-/tmp/app.log}" |
  "${logfmt[@]}"
