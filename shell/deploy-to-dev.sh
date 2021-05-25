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
bento="bento1a"
version="latest"
namespace="$APPNAME--$bento"
environment=${DEPLOY_TO_DEV_ENVIRONMENT:-"development"}

if ! command -v kubecfg >/dev/null; then
  info "Hint: brew install kubecfg"
  fatal "kubecfg must be installed"
fi

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

# Only run devenv checks when we're now showing manifests
if [[ $action != "show" ]]; then
  # Ensure the devenv is installed
  if ! command -v devenv >/dev/null 2>&1; then
    fatal "devenv was not found in PATH, please install from https://github.com/getoutreach/dev-environment"
  fi

  # Ensure the devenv is running
  if ! devenv status --quiet; then
    fatal "devenv doesn't appear to be in a running state, run 'devenv status' for more information"
  fi
fi

kubecfg \
  --jurl http://k8s-clusters.outreach.cloud/ \
  --jurl https://raw.githubusercontent.com/getoutreach/jsonnet-libs/master \
  -n "$namespace" \
  --context "dev-environment" "$action" "$(get_repo_directory)/deployments/$APPNAME/$APPNAME.jsonnet" \
  -V cluster="development.us-west-2" \
  -V namespace="$namespace" \
  -V environment="$environment" \
  -V version="$version" \
  -V bento="$bento" \
  -V dev_email="${DEV_EMAIL:-$(git config user.email)}" \
  -V host="bento1a.outreach-dev.com"
