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

if [[ -z $TESTING_DO_NOT_BUILD ]]; then
  # shellcheck source=../../lib/buildx.sh
  source "${LIB_DIR}/buildx.sh"
fi

# shellcheck source=../../lib/logging.sh
source "${LIB_DIR}/logging.sh"

# shellcheck source=../../lib/yaml.sh
source "${LIB_DIR}/yaml.sh"

# get_image_field is a helper to return a field from the manifest
# for a given image. It will return an empty string if the field
# is not set.
#
# Arguments:
#   $1 - image name
#   $2 - field name
#   $3 - type (array or string) (default: string)
#   $4 - manifest file (default: $MANIFEST)
get_image_field() {
  local image="$1"
  local field="$2"

  # type can be 'array' or 'string'. Array values are
  # returned as newline separated values, string values
  # are returned as a single string. A string value can
  # be strings, ints, bools, etc.
  local type=${3:-string}
  local manifest=${4:-$MANIFEST}

  local filter="$(yaml_construct_object_filter "$image" "$field")"
  if [[ $type == "array" ]]; then
    yaml_get_array "$filter" "$manifest"
    return
  elif [[ $type == "string" ]]; then
    yaml_get_field "$filter" "$manifest"
    return
  else
    error "Unknown type '$type' for get_image_field"
    fatal "Expected 'array' or 'string'"
  fi
}

# build_and_push_image builds and pushes a docker image to
# the configured registry
build_and_push_image() {
  local image="$1"

  # push determines if we should push the image to the registry or not.
  local push=false

  # Platforms to build this image for, expected format (in YAML):
  # platforms:
  #   - linux/arm64
  #   - linux/amd64
  #
  # See buildkit docs: https://github.com/docker/buildx#building-multi-platform-images
  mapfile -t platforms < <(get_image_field "$image" "platforms" "array")
  if [[ -z $platforms ]]; then
    platforms=("linux/arm64" "linux/amd64")
  fi

  # Expose secrets to a docker container, expected format (in YAML):
  #
  # secrets:
  #   - id=secretID,env=ENV_VAR
  #
  # See docker docs:
  # https://docs.docker.com/develop/develop-images/build_enhancements/#new-docker-build-secret-information
  mapfile -t secrets < <(get_image_field "$image" "secrets" "array")
  if [[ -z $secrets ]]; then
    secrets=("id=npmtoken,env=NPM_TOKEN")
  fi

  local imageRegistry="$(get_box_field 'devenv.imageRegistry')"

  # Where to push the image. This can be overridden in the manifest
  # with the field .pushTo. If not set, we'll use the imageRegistry
  # from the box configuration and the name of the image in devenv.yaml
  # as the repository. If this is not the main image (appName), we'll
  # append the appName to the repository to keep the images isolated
  # to this repository.
  local remote_image_name=$(get_image_field "$image" "pushTo")
  if [[ -z $remote_image_name ]]; then
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

  local args=(
    "--ssh" "default"
    "--progress=plain" "--file" "$dockerfile"
    "--build-arg" "VERSION=${VERSION}"
  )

  for secret in "${secrets[@]}"; do
    args+=("--secret" "$secret")
  done

  # Argument format: os/arch,os/arch
  local platformArgumentString=""
  for platform in "${platforms[@]}"; do
    if [[ -n $platformArgumentString ]]; then
      platformArgumentString+=","
    fi
    platformArgumentString+="$platform"
  done
  args+=("--platform" "$platformArgumentString")

  # tags are the tags to apply to the image. If we're on a git tag,
  # we'll tag the image with that tag and latest. Otherwise, we'll just
  # build a latest image for the name "$image" (the name of the image as
  # shown in the manifest) instead.
  local tags=()
  if [[ -n $CIRCLE_TAG ]]; then
    tags+=("$remote_image_name:$CIRCLE_TAG" "$remote_image_name:latest")

    # When on a tag, we should also push the image to the registry. Also
    # set push to true so that we log the right message.
    args+=("--push")
    push=true
  else
    tags+=("$image")
  fi
  for tag in "${tags[@]}"; do
    args+=("--tag" "$tag")
  done

  # If we're not the main image, the build context should be
  # the image directory instead.
  local buildContext="$(get_image_field "$image" "buildContext")"
  if [[ -z $buildContext ]]; then
    buildContext="."
    if [[ $APPNAME != "$image" ]]; then
      buildContext="$(get_repo_directory)/deployments/$image"
    fi
  fi
  args+=("$buildContext")

  if [[ $push == true ]]; then
    echo "🔨 Building and Pushing Docker Image"
  else
    echo "🔨 Building Docker Image for Validation"
  fi
  (
    if [[ $OSTYPE == "linux-gnu"* ]]; then
      docker buildx --builder devbase build "${args[@]}"
    else
      docker buildx build "${args[@]}"
      docker buildx prune
    fi
  )
}

# HACK(jaredallard): Skips building images if TESTING_DO_NOT_BUILD is set. We
# should break out the functions from this script instead.
if [[ -z $TESTING_DO_NOT_BUILD ]]; then
  if [[ ! -f $MANIFEST ]]; then
    error "Manifest file '$MANIFEST' required for building Docker images"
    fatal "See https://github.com/getoutreach/devbase#building-docker-images for details"
  fi

  # Build and (on tags: push) all images in the manifest
  mapfile -t images < <(yq -r 'keys[]' "$MANIFEST")
  for image in "${images[@]}"; do
    build_and_push_image "$image"
  done
fi
