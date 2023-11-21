#!/usr/bin/env bash
# Helper functions for Docker image generation.

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"

# shellcheck source=./logging.sh
source "${DIR}/logging.sh"

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

# run_docker is a wrapper for the docker command, but it prints out the
# command (via set -x).
run_docker() {
  set -x
  docker "$@"
  set +x
}
