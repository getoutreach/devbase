#!/usr/bin/env bash
#
# wrapper arround jsonnet for rendering files
SCRIPTS_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
# shellcheck source=./lib/bootstrap.sh
source "$SCRIPTS_DIR/lib/bootstrap.sh"

APPNAME="$(get_app_name)"

# shellcheck source=./lib/logging.sh
source "$SCRIPTS_DIR/lib/logging.sh"

action=$1

export DEVENV_DEPLOY_BENTO="bento1a"
export DEVENV_DEPLOY_VERSION="${DEPLOY_TO_DEV_VERSION:-"latest"}"
export DEVENV_DEPLOY_NAMESPACE="$APPNAME--$DEVENV_DEPLOY_BENTO"
export DEVENV_DEPLOY_ENVIRONMENT="${DEPLOY_TO_DEV_ENVIRONMENT:-"development"}"
export DEVENV_DEPLOY_HOST="bento1a.outreach-dev.com"

showHelp() {
  echo "usage: deploy-to-dev.sh <action>"
  echo ""
  echo "action: show, update, delete"
}

if [[ $1 == "--help" ]] || [[ $1 == "-h" ]]; then
  showHelp
  exit
fi

if [[ -z $action ]]; then
  showHelp
  exit 1
fi

DEVENV_BIN="${DEVENV_BIN:-devenv}"

"$DEVENV_BIN" --skip-update k --namespace "$DEVENV_DEPLOY_NAMESPACE" delete jobs --all

exec "$SCRIPTS_DIR/build-jsonnet.sh" "$action"
