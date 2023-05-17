#!/usr/bin/env bash
#
# wrapper around jsonnet for rendering files
SCRIPTS_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
# shellcheck source=./lib/bootstrap.sh
source "$SCRIPTS_DIR/lib/bootstrap.sh"

# shellcheck source=./lib/logging.sh
source "$SCRIPTS_DIR/lib/logging.sh"

action=$1

appName="${DEVENV_DEPLOY_APPNAME:-$(get_app_name)}"
bento="${DEVENV_DEPLOY_BENTO:-"bento1a"}"
channel="${DEVENV_DEPLOY_CHANNEL:-"devenv"}"
namespace="${DEVENV_DEPLOY_NAMESPACE:-$appName--$bento}"
version="${DEVENV_DEPLOY_VERSION:-"latest"}"
environment="${DEVENV_DEPLOY_ENVIRONMENT:-"development"}"
host="${DEVENV_DEPLOY_HOST:-"bento1a.outreach-dev.com"}"
email="${DEV_EMAIL:-$(git config user.email)}"
appImageRegistry="${DEVENV_DEPLOY_IMAGE_REGISTRY:-"gcr.io/outreach-docker"}"

kubecfg \
  --jurl http://k8s-clusters.outreach.cloud/ \
  --jurl https://raw.githubusercontent.com/getoutreach/jsonnet-libs/master \
  -n "$namespace" \
  --context "dev-environment" "$action" "$(get_repo_directory)/deployments/$appName/$appName.jsonnet" \
  -V cluster="development.us-west-2" \
  -V region="us-west-2" \
  -V namespace="$namespace" \
  -V environment="$environment" \
  -V version="$version" \
  -V bento="$bento" \
  -V channel="$channel" \
  -V dev_email="$email" \
  -V host="$host" \
  -V appImageRegistry="$appImageRegistry"
