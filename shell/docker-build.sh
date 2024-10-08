#!/usr/bin/env bash
# Builds a Docker image for the current application in the dev environment.

set -eo pipefail

LIB_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)/lib"

# shellcheck source=./lib/bootstrap.sh
source "${LIB_DIR}/bootstrap.sh"

# shellcheck source=./lib/box.sh
source "${LIB_DIR}/box.sh"

# shellcheck source=./lib/logging.sh
source "${LIB_DIR}/logging.sh"

# shellcheck source=./lib/docker.sh
source "${LIB_DIR}/docker.sh"

appName="$(get_app_name)"

if [[ -z $DOCKERFILE ]]; then
  DOCKERFILE="deployments/$appName/Dockerfile"
fi

info "Building docker image for ${appName} …"

warn "If you run into credential issues, ensure that your key is in your SSH agent (ssh-add <ssh-key-path>)"

tags=()

imageRegistries="$(get_docker_push_registries)"

for imageRegistry in $imageRegistries; do
  tags+=("--tag" "$imageRegistry/$appName")
done

# Assume that $APP_VERSION is set in the environment
# shellcheck disable=SC2086
DOCKER_BUILDKIT=1 docker build \
  --ssh default "${tags[@]}" \
  -f "$DOCKERFILE" \
  . \
  --build-arg VERSION="$APP_VERSION" \
  $DOCKER_BUILD_EXTRA_ARGS
