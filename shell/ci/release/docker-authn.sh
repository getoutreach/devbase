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
CIRCLECI_DIR="${DIR}/../../circleci"
LIB_DIR="${DIR}/../../lib"

# shellcheck source=../../lib/logging.sh
source "${LIB_DIR}/logging.sh"

# shellcheck source=../../circleci/install_gh.sh
source "${CIRCLECI_DIR}/install_gh.sh"

# In order to get the box config, we need to authenticate with GitHub
# shellcheck source=../auth/github.sh
source "${AUTH_DIR}/github.sh"

git config --global --remove-section url."ssh://git@github.com"
GH_NO_UPDATE_NOTIFIER=true gh auth setup-git

# shellcheck source=../../lib/box.sh
source "${LIB_DIR}/box.sh"

download_box

# shellcheck source=../../lib/docker.sh
source "${LIB_DIR}/docker.sh"

pullRegistry="${DOCKER_PULL_REGISTRY:-$(get_docker_pull_registry)}"
pushRegistries="${DOCKER_PUSH_REGISTRIES:-$(get_docker_push_registries)}"

registries=$(echo "$pullRegistry $pushRegistries" | tr ' ' '\n' | sort --unique | tr '\n' ' ')

info "Authenticating with Docker registries"

for crURL in $registries; do
  case $crURL in
  gcr.io/*)
    info_sub "GCR"
    # shellcheck source=../auth/gcr.sh
    source "${AUTH_DIR}/gcr.sh"
    ;;
  *.amazonaws.com | *.amazonaws.com/*)
    info_sub "AWS ECR"
    # shellcheck source=../auth/aws-ecr.sh
    source "${AUTH_DIR}/aws-ecr.sh"
    ;;
  *)
    warn "No authentication script found for registry: $crURL"
    ;;
  esac
done
