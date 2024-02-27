#!/usr/bin/env bash
# shellcheck disable=SC2128,SC2155
# Builds a docker image, and pushes it if it's in CircleCI
set -e

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
LIB_DIR="${DIR}/../../lib"

# shellcheck source=../../lib/bootstrap.sh
source "${LIB_DIR}/bootstrap.sh"

# shellcheck source=../../lib/box.sh
source "${LIB_DIR}/box.sh"

APPNAME="$(get_app_name)"
VERSION="$(make --no-print-directory version)"
MANIFEST="$(get_repo_directory)/deployments/docker.yaml"

# shellcheck source=../../lib/buildx.sh
source "${LIB_DIR}/buildx.sh"

# shellcheck source=../../lib/logging.sh
source "${LIB_DIR}/logging.sh"

# shellcheck source=../../lib/docker.sh
source "${LIB_DIR}/docker.sh"

# build_and_save_image builds and optionally saves the image to disk.
build_and_save_image() {
  local image="$1"

  local dockerfile="deployments/$image/Dockerfile"
  if [[ ! -e $dockerfile ]]; then
    echo "No dockerfile found at path: $dockerfile. Skipping."
    return
  fi

  mkdir -p docker-images

  local args
  args="$(docker_buildx_args "$APPNAME" "$VERSION" "$image" "$dockerfile" "$IMAGE_ARCH")"

  echo "🔨 Building and saving Docker image to disk"
  (
    if [[ $OSTYPE == "linux-gnu"* ]]; then
      # We want globbing/word splitting on the args
      # shellcheck disable=SC2086
      run_docker buildx --builder devbase build $args
      run_docker buildx prune --force --keep-storage 6GB
    else
      # We want globbing/word splitting on the args
      # shellcheck disable=SC2086
      run_docker buildx build $args
    fi
  )
}

if [[ ! -f $MANIFEST ]]; then
  error "Manifest file '$MANIFEST' required for building Docker images"
  fatal "See https://github.com/getoutreach/devbase#building-docker-images for details"
fi

# Build and save all images in the manifest
mapfile -t images < <(docker_manifest_images "$MANIFEST")
for image in "${images[@]}"; do
  build_and_save_image "$image"
done
