#!/usr/bin/env bash
# Builds a Docker image for the current application in the dev environment.
#
# External environment variables accepted:
#
# * `APP_VERSION` (REQUIRED)
# * `DOCKERFILE` (defaults to `deployments/$app/Dockerfile`)
# * `IMAGE_NAME` (optional, if not set defaults to appName)
# * `DOCKER_BUILD_EXTRA_ARGS` (extra arguments to `docker build`,
#   defaults to empty string)

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

if [[ -z ${APP_VERSION:-} ]]; then
  fatal "Please specify APP_VERSION"
fi

appName="$(get_app_name)"

if [[ -z ${DOCKERFILE:-} ]]; then
  DOCKERFILE="deployments/$appName/Dockerfile"
fi

# If IMAGE_NAME is not set, default to appName
if [[ -z ${IMAGE_NAME:-} ]]; then
  IMAGE_NAME="$appName"
fi

info "Building docker image for ${IMAGE_NAME} …"

warn "If you run into credential issues, ensure that your key is in your SSH agent (ssh-add <ssh-key-path>)"

tags=()

imageRegistries="$(get_docker_push_registries)"

for imageRegistry in $imageRegistries; do
  if [[ "$IMAGE_NAME" == "$appName" ]]; then
    # If image name equals app name, use the default format: imageRegistry/appName
    tags+=("--tag" "$imageRegistry/$appName")
    info "tag: $imageRegistry/$appName"
  else
    # Otherwise, use: imageRegistry/appName/imageName
    tags+=("--tag" "$imageRegistry/$appName/$IMAGE_NAME")
    info "tag: $imageRegistry/$appName/$IMAGE_NAME"
  fi
done

# Assume that $APP_VERSION is set in the environment
# shellcheck disable=SC2086
DOCKER_BUILDKIT=1 docker build \
  --ssh default "${tags[@]}" \
  -f "$DOCKERFILE" \
  . \
  --build-arg VERSION="${APP_VERSION:-}" \
  ${DOCKER_BUILD_EXTRA_ARGS:-}
