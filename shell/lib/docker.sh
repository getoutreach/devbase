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

# Generates Docker CLI arguments for building an image.
#
# docker_buildx_args(app_name, version, image, dockerfile[, arch]) -> arg string
docker_buildx_args() {
  local app_name="$1"
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

  # tags are the tags to apply to the image. If we're on a git tag,
  # we'll tag the image with that tag and latest. Otherwise, we'll just
  # build a latest image for the name "$image" (the name of the image as
  # shown in the manifest) instead.
  local tags=()
  if [[ -n $CIRCLE_TAG ]]; then
    tags+=("$image")
    if [[ -n $arch ]]; then
      local remote_image_name
      remote_image_name="$(determine_remote_image_name "$app_name" "$(get_box_field 'devenv.imageRegistry')" "$image")"
      tags+=("$remote_image_name:latest-$arch" "$remote_image_name:$version-$arch")
    fi
  fi
  for tag in "${tags[@]}"; do
    args+=("--tag" "$tag")
  done

  args+=("--output" "type=docker,dest=./docker-images/$image-$(uname -m).tar")

  # If we're not the main image, the build context should be
  # the image directory instead.
  local buildContext
  buildContext="$(get_image_field "$image" "buildContext")"
  if [[ -z $buildContext ]]; then
    buildContext="."
    if [[ $app_name != "$image" ]]; then
      buildContext="$(get_repo_directory)/deployments/$image"
    fi
  fi
  args+=("$buildContext")

  echo "${args[@]}"
}

# Where to push the image. This can be overridden in the manifest
# with the field .pushTo. If not set, we'll use the imageRegistry
# from the box configuration and the name of the image in devenv.yaml
# as the repository. If this is not the main image (app_name), we'll
# append the app_name to the repository to keep the images isolated
# to this repository.
#
# determine_remote_image_name(app_name, image_registry, image) -> remote_image_name
determine_remote_image_name() {
  local app_name="$1"
  local image_registry="$2"
  local image="$3"
  local remote_image_name

  remote_image_name=$(get_image_field "$image" "pushTo")
  if [[ -z $remote_image_name ]]; then
    remote_image_name="$image_registry/$image"

    # If we're not the main image, then we should prefix the image name with the
    # app name, so that we can easily identify the image's source.
    if [[ $image != "$app_name" ]]; then
      remote_image_name="$image_registry/$app_name/$image"
    fi
  fi

  echo "$remote_image_name"
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
