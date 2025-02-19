#!/usr/bin/env bash
#
# Authenticate with supported Docker registries.
# Resolves in the following prcedence order (empty values are ignored):
# 1. From environment variables:
#    a. DOCKER_PULL_REGISTRY | DOCKER_PUSH_REGISTRIES
#    b. gobox-defined: BOX_DOCKER_PULL_IMAGE_REGISTRY | BOX_DOCKER_PUSH_IMAGE_REGISTRIES
# 2. From box config:
#    a. docker.imagePullRegistry | docker.imagePushRegistries
#    b. devenv.imageRegistry

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
AUTH_DIR="${DIR}/../auth"
LIB_DIR="${DIR}/../../lib"
DOCKER_AUTH_DIR="${LIB_DIR}/docker/authn"
ROOT_DIR="${DIR}/../../.."

# shellcheck source=../../lib/logging.sh
source "${LIB_DIR}/logging.sh"

source "${LIB_DIR}/mise.sh"

tool_version() {
  local tool="$1"
  grep "^$tool:" "$ROOT_DIR"/versions.yaml | awk '{print $2}'
}

info "Ensuring that 'gh' is installed"

install_tool_with_mise gh "$(tool_version gh)"
install_tool_with_mise gojq "$(tool_version gojq)"

info "🔓 Authenticating to GitHub"

# In order to get the box config, we need to authenticate with GitHub
# shellcheck source=../auth/github.sh
source "${AUTH_DIR}/github.sh"
# We need to set up SSH to ensure that we can access private
# repositories when building the Docker images
# shellcheck source=../auth/ssh.sh
source "${AUTH_DIR}/ssh.sh"

git config --global --remove-section url."ssh://git@github.com"
GH_NO_UPDATE_NOTIFIER=true gh auth setup-git

# shellcheck source=../../lib/box.sh
source "${LIB_DIR}/box.sh"

download_box

# shellcheck source=../../lib/docker.sh
source "${LIB_DIR}/docker.sh"

# shellcheck source=../../lib/docker/authn/aws-ecr.sh
source "${DOCKER_AUTH_DIR}/aws-ecr.sh"

# shellcheck source=../../lib/docker/authn/gcr.sh
source "${DOCKER_AUTH_DIR}/gcr.sh"

pullRegistry="${DOCKER_PULL_REGISTRY:-$(get_docker_pull_registry)}"
pushRegistries="${DOCKER_PUSH_REGISTRIES:-$(get_docker_push_registries)}"

registries=$(echo "$pullRegistry $pushRegistries" | tr ' ' '\n' | sort --unique | tr '\n' ' ')

info "🔓 Authenticating to Docker registries"

for crURL in $registries; do
  case $crURL in
  gcr.io/*)
    info_sub "🔓 GCR ($crURL)"
    gcr_auth "$GCLOUD_SERVICE_ACCOUNT"
    ;;
  *.amazonaws.com | *.amazonaws.com/*)
    ecr_auth "$crURL"
    ;;
  *)
    warn "No authentication script found for registry: $crURL"
    ;;
  esac
done
