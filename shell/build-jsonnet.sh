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

bento="${DEVENV_DEPLOY_BENTO:-"bento1a"}"
namespace="${DEVENV_DEPLOY_NAMESPACE:-$APPNAME--$bento}"
version="${DEVENV_DEPLOY_VERSION:-"latest"}"
environment="${DEVENV_DEPLOY_ENVIRONMENT:-"development"}"
host="${DEVENV_DEPLOY_HOST:-"bento1a.outreach-dev.com"}"
email="${DEVENV_DEPLOY_DEV_EMAIL:-$(git config user.email)}"

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
  -V dev_email="$email" \
  -V host="$host"
