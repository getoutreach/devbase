#!/usr/bin/env bash
# Helper functions for Docker image generation.

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
YQ="${DIR}/../yq.sh"

# shellcheck source=./logging.sh
source "${DIR}/logging.sh"

# shellcheck source=./bootstrap.sh
source "${DIR}/bootstrap.sh"

# shellcheck source=./yaml.sh
source "${DIR}/yaml.sh"

# shellcheck source=./box.sh
source "${DIR}/box.sh"

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
  local filter

  filter="$(yaml_construct_object_filter "$image" "$field")"
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

# Returns a space-separated list of image registries to push to.
# `BOX_DOCKER_PUSH_IMAGE_REGISTRIES` is the environment variable that
# takes preference over every other method of determining the registries.
# `DOCKER_PUSH_REGISTRIES` is the environment variable that can be set
# by the CircleCI job via the `push_registries` parameter. If none of
# these are set, the box field `docker.imagePushRegistries`` is used.
get_docker_push_registries() {
  local imageRegistries
  if [[ -n $BOX_DOCKER_PUSH_IMAGE_REGISTRIES ]]; then
    imageRegistries="$BOX_DOCKER_PUSH_IMAGE_REGISTRIES"
  else
    imageRegistries="${DOCKER_PUSH_REGISTRIES:-$(get_box_array 'docker.imagePushRegistries')}"
  fi

  if [[ -z $imageRegistries ]]; then
    # Fall back to the old box field
    imageRegistries="$(get_box_field 'devenv.imageRegistry')"
  fi

  echo "$imageRegistries"
}

# Returns the registry to pull images from. This is determined by either
# the environment variable BOX_DOCKER_PULL_IMAGE_REGISTRY or one of the
# following box fields:
# * docker.imagePullRegistry
# * devenv.imageRegistry
get_docker_pull_registry() {
  if [[ -n $BOX_DOCKER_PULL_IMAGE_REGISTRY ]]; then
    echo "$BOX_DOCKER_PULL_IMAGE_REGISTRY"
  else
    local pullRegistry
    pullRegistry="$(get_box_field 'docker.imagePullRegistry')"
    if [[ -z $pullRegistry ]]; then
      pullRegistry="$(get_box_field 'devenv.imageRegistry')"
    fi

    echo "$pullRegistry"
  fi
}

# Generates Docker CLI arguments for building an image.
#
# docker_buildx_args(appName, version, image, dockerfile[, arch]) -> arg string
docker_buildx_args() {
  local appName="$1"
  local version="$2"
  local image="$3"
  local dockerfile="$4"
  local arch="$5"

  # Platforms to build this image for, expected format (in YAML):
  # platforms:
  #   - linux/arm64
  #   - linux/amd64
  #
  # See buildkit docs: https://github.com/docker/buildx#building-multi-platform-images
  mapfile -t platforms < <(get_image_field "$image" "platforms" "array")
  if [[ ${#platforms[@]} -eq 0 ]]; then # no platforms found
    if [[ -n $arch ]]; then
      platforms=("linux/${arch}")
    else
      platforms=("linux/arm64" "linux/amd64")
    fi
  fi

  # Expose secrets to a docker container, expected format (in YAML):
  #
  # secrets:
  #   - id=secretID,env=ENV_VAR
  #
  # See docker docs:
  # https://docs.docker.com/develop/develop-images/build_enhancements/#new-docker-build-secret-information
  mapfile -t secrets < <(get_image_field "$image" "secrets" "array")
  if [[ ${#secrets[@]} -eq 0 ]]; then # no secrets found
    secrets=("id=npmtoken,env=NPM_TOKEN")
  fi

  local args=(
    "--ssh" "default"
    "--progress=plain" "--file" "$dockerfile"
    "--build-arg" "VERSION=${version}"
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

  # tags are the tags to apply to the image. We always tag the image with "latest" tag
  # for each arch and the version tag the same. Tags are only applied if we intend to push
  # the image to registries later.
  if [[ $(will_push_images) == "true" ]]; then
    local tags=("$image")

    local remoteImageName
    local pushRegistries
    pushRegistries="$(get_docker_push_registries)"
    for pushRegistry in $pushRegistries; do
      remoteImageName="$(determine_remote_image_name "$appName" "$pushRegistry" "$image")"
      if [[ -n $arch ]]; then
        tags+=("$remoteImageName:latest-$arch" "$remoteImageName:$version-$arch")
      fi
    done

    for tag in "${tags[@]}"; do
      args+=("--tag" "$tag")
    done
  fi

  args+=("--output" "type=docker,dest=./docker-images/$image-$(uname -m).tar")

  # If we're not the main image, the build context should be
  # the image directory instead.
  local buildContext
  buildContext="$(get_image_field "$image" "buildContext")"
  if [[ -z $buildContext ]]; then
    buildContext="."
    if [[ $appName != "$image" ]]; then
      buildContext="$(get_repo_directory)/deployments/$image"
    fi
  fi
  args+=("$buildContext")

  echo "${args[@]}"
}

# Where to push the image. This can be overridden in the manifest
# with the field .pushTo. If not set, we'll use the imageRegistry
# from the box configuration and the name of the image in devenv.yaml
# as the repository. If this is not the main image (appName), we'll
# append the appName to the repository to keep the images isolated
# to this repository.
#
# determine_remote_image_name(appName, imageRegistry, image) -> remoteImageName
determine_remote_image_name() {
  local appName="$1"
  local imageRegistry="$2"
  local image="$3"
  local remoteImageName

  remoteImageName=$(get_image_field "$image" "pushTo")
  if [[ -z $remoteImageName ]]; then
    remoteImageName="$imageRegistry/$image"

    # If we're not the main image, then we should prefix the image name with the
    # app name, so that we can easily identify the image's source.
    if [[ $image != "$appName" ]]; then
      remoteImageName="$imageRegistry/$appName/$image"
    fi
  fi

  echo "$remoteImageName"
}

# run_docker is a wrapper for the docker command, but it prints out the
# command (via set -x).
run_docker() {
  set -x
  docker "$@"
  set +x
}

# docker_manifest_images retrieves the list of images from a manifest file.
docker_manifest_images() {
  local manifest="$1"

  "$YQ" -r 'keys[]' "$manifest"
}

# will_push_images determines if current pipeline is configured to actually push image to a registry.
# Decision is made based on global variables "VERSIONING_SCHEME", "DRY_RUN" and "CIRCLE_TAG".
will_push_images() {
  local mode="$VERSIONING_SCHEME"
  if [[ -z $mode ]]; then
    mode="semver"
  fi

  local result="false"

  # If we're in SemVer mode and CI is running off of a tag -- we push images
  if [[ $mode == "semver" && -n $CIRCLE_TAG ]]; then
    result="true"
  # If we're in SHA release mode and DRY_RUN was not explicitly set to true -- we push images
  elif [[ $mode == "sha" && $DRY_RUN != "true" ]]; then
    result="true"
  fi

  echo "$result"
}
