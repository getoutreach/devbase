#!/usr/bin/env bash
#
# wrapper around jsonnet for rendering files

set -eo pipefail

SCRIPTS_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
# shellcheck source=./lib/bootstrap.sh
source "$SCRIPTS_DIR/lib/bootstrap.sh"

# shellcheck source=./lib/docker.sh
source "$SCRIPTS_DIR/lib/docker.sh"

# shellcheck source=./lib/logging.sh
source "$SCRIPTS_DIR/lib/logging.sh"

# shellcheck source=./lib/mise.sh
source "$SCRIPTS_DIR/lib/mise.sh"

# Cache a local copy of the `jsonnet-libs` directory on disk if it doesn't yet exist. Do this
# because it helps us avoid accessing jsonnet-libs via raw.githubusercontent.com, which has
# aggressive rate limits that we can easily hit. Estimated API usage reduction is +10x since before
# we'd make 1 request per file (15+ *sonnet files), now we clone at most once per run.
JSONNET_LIBS_REPO="$HOME/.outreach/.cache/jsonnet-libs"

if [[ -d $JSONNET_LIBS_REPO ]]; then
  pushd "$JSONNET_LIBS_REPO" >/dev/null || fatal "Could not find jsonnet-libs cache dir"
  git pull --quiet
  popd >/dev/null || fatal "Could not change directory out of jsonnet-libs cache dir"
else
  mkdir -p "$(dirname "$JSONNET_LIBS_REPO")"
  git clone --quiet --single-branch git@github.com:getoutreach/jsonnet-libs "$JSONNET_LIBS_REPO" >/dev/null
fi

action=$1

appName="${DEVENV_DEPLOY_APPNAME:-$(get_app_name)}"
bento="${DEVENV_DEPLOY_BENTO:-"bento1a"}"
channel="${DEVENV_DEPLOY_CHANNEL:-"devenv"}"
namespace="${DEVENV_DEPLOY_NAMESPACE:-$appName--$bento}"
version="${DEVENV_DEPLOY_VERSION:-"latest"}"
environment="${DEVENV_DEPLOY_ENVIRONMENT:-"development"}"
host="${DEVENV_DEPLOY_HOST:-"bento1a.outreach-dev.com"}"
email="${DEV_EMAIL:-$(git config user.email || echo 'devbase@outreach.io')}"
appImageRegistry="${DEVENV_DEPLOY_IMAGE_REGISTRY:-"$(get_docker_pull_registry)"}"

"$(find_tool kubecfg)" \
  --jpath "$JSONNET_LIBS_REPO" \
  --jurl http://k8s-clusters.outreach.cloud/ \
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
