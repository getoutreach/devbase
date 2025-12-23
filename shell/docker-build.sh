#!/usr/bin/env bash
# Builds a Docker image for the current application in the dev environment.
#
# External environment variables accepted:
#
# * `APP_VERSION` (REQUIRED)
# * `DOCKERFILE` (defaults to `deployments/$app/Dockerfile`)
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
  info "Building default Docker image for $appName …"
else
  info "Building Docker image for $appName ($DOCKERFILE) …"
fi

warn "If you run into credential issues, ensure that your key is in your SSH agent (ssh-add <ssh-key-path>)"

tags=()

if [[ -n ${BASE_IMAGE:-} ]]; then
  info_sub "Using custom image name: $BASE_IMAGE"
  tags+=("--tag" "$BASE_IMAGE")
else
  imageRegistries="$(get_docker_push_registries)"

  for imageRegistry in $imageRegistries; do
    tags+=("--tag" "$imageRegistry/$appName")
  done
fi

# Assume that $APP_VERSION is set in the environment
# shellcheck disable=SC2086
DOCKER_BUILDKIT=1 docker build \
  --ssh default "${tags[@]}" \
  -f "$DOCKERFILE" \
  . \
  --build-arg VERSION="${APP_VERSION:-}" \
  ${DOCKER_BUILD_EXTRA_ARGS:-}
