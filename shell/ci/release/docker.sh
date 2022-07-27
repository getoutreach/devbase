#!/usr/bin/env bash
# shellcheck disable=SC2128,SC2155
# Builds a docker image, and pushes it if it's in CircleCI
set -e

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
LIB_DIR="${DIR}/../../lib"
SEC_DIR="${DIR}/../../security"
TWIST_SCAN_DIR="${SEC_DIR}/prismaci"

# shellcheck source=../../lib/bootstrap.sh
source "${LIB_DIR}/bootstrap.sh"

# shellcheck source=../../lib/box.sh
source "${LIB_DIR}/box.sh"

APPNAME="$(get_app_name)"
VERSION="$(make version)"
MANIFEST="$(get_repo_directory)/deployments/docker.yaml"

# shellcheck source=../../lib/buildx.sh
source "${LIB_DIR}/buildx.sh"

# shellcheck source=../../lib/logging.sh
source "${LIB_DIR}/logging.sh"

# get_image_field returns a field from the manifest of the image
get_image_field() {
  local field="$1"
  yq -r ".[\"$name\"]$field" "$MANIFEST"
}

# build_and_push_image builds and pushes a docker image to
# the configured registry
build_and_push_image() {
  local image="$1"

  # Platforms to build this image for, expected format (in YAML):
  # platforms:
  #   - linux/arm64
  #   - linux/amd64
  #
  # See buildkit docs: https://github.com/docker/buildx#building-multi-platform-images
  mapfile -t platforms < <(get_image_field '.platforms')
  if [[ -z $platforms ]] || [[ $platforms == "null" ]]; then
    platforms=("linux/arm64" "linux/amd64")
  fi

  # Expose secrets to a docker container, expected format (in YAML):
  #
  # secrets:
  #   - id=secretID,env=ENV_VAR
  #
  # See docker docs:
  # https://docs.docker.com/develop/develop-images/build_enhancements/#new-docker-build-secret-information
  mapfile -t secrets < <(get_image_field '.secrets')
  if [[ -z $secrets ]] || [[ $secrets == "null" ]]; then
    secrets=("id=npmtoken,env=NPM_TOKEN")
  fi

  local imageRegistry="$(get_box_field '.devenv.imageRegistry')"

  # Where to push the image. This can be overridden in the manifest
  # with the field .pushTo. If not set, we'll use the imageRegistry
  # from the box configuration and the name of the image in devenv.yaml
  # as the repository. If this is not the main image (appName), we'll
  # append the appName to the repository to keep the images isolated
  # to this repository.
  local remote_image_name=$(get_image_field '.pushTo')
  if [[ -z $remote_image_name ]] || [[ $remote_image_name == "null" ]]; then
    local remote_image_name="$imageRegistry/$image"

    # If we're not the main image, then we should prefix the image name with the
    # app name, so that we can easily identify the image's source.
    if [[ $image != "$APPNAME" ]]; then
      remote_image_name="$imageRegistry/$APPNAME/$image"
    fi
  fi

  local dockerfile="deployments/$image/Dockerfile"
  if [[ ! -e $dockerfile ]]; then
    echo "No dockerfile found at path: $dockerfile. Skipping."
    return
  fi

  args=(
    "--ssh" "default"
    "--progress=plain" "--file" "$dockerfile"
    "--build-arg" "VERSION=${VERSION}"
  )

  for secret in "${secrets[@]}"; do
    args+=("--secret" "$secret")
  done

  # Argument format: os/arch,os/arch
  platformArgumentString=""
  for platform in "${platforms[@]}"; do
    if [[ -n $platformArgumentString ]]; then
      platformArgumentString+=","
    fi
    platformArgumentString+="$platform"
  done

  # If we're not the main image, the build context should be
  # the image directory instead.
  buildContext="$(get_image_field '.buildContext')"
  if [[ -z $buildContext ]] || [[ $buildContext == "null" ]]; then
    buildContext="."
    if [[ $APPNAME != "$image" ]]; then
      buildContext="$(get_repo_directory)/deployments/$image"
    fi
  fi

  # Build a quick native image and load it into docker cache for security scanning
  # Scan reports for release images are also uploaded to OpsLevel
  # (test image reports only available on PR runs as artifacts).
  info "Building Docker Image (for scanning)"
  (
    set -x
    docker buildx build "${args[@]}" -t "$image" --load "$buildContext"
  )

  info "🔐 Scanning docker image for vulnerabilities"
  "${TWIST_SCAN_DIR}/twist-scan.sh" "$image" || echo "Warning: Failed to scan image" >&2

  if [[ -n $CIRCLE_TAG ]]; then
    echo "🔨 Building and Pushing Docker Image (production)"
    (
      set -x
      docker buildx build "${args[@]}" --platform "$platformArgumentString" \
        -t "$remote_image_name:$VERSION" -t "$remote_image_name:latest" --push \
        "$buildContext"
    )
  fi
}

mapfile -t images < <(yq -r 'keys[]' "$MANIFEST")
for image in "${images[@]}"; do
  build_and_push_image "$image"
done
