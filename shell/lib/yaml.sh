#!/usr/bin/env bash
# yaml is a general purpose bash yaml parsing library

YQ="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"/../yq.sh

# yaml_get_array returns a newline separated list of values
# from a yaml array. If a value is not set, it will return
# an empty string.
#
# To convert into a bash array, use the following:
#
#  mapfile -t my_array < <(yaml_get_array "$filter" "$file")
#
# $1 yq filter
# $2 yaml file
yaml_get_array() {
  local filter="$1"
  local file="$2"
  "$YQ" -r "$filter | .[]?" "$file"
}

# yaml_construct_object_filter creates a yq filter for all
# arguments passed to access a field on an object. For example,
# if you have a yaml file with the following contents:
#
# foo:
#   bar:
#     baz: 1
#
# yaml_construct_object_filter foo bar baz will return
# .["foo"]["bar"]["baz"]
yaml_construct_object_filter() {
  local filter="."
  for arg in "$@"; do
    filter+="[\"$arg\"]"
  done
  echo "$filter"
}

# yaml_get_field returns a value from a yaml file. If the
# value is not set, or is null, an empty string is returned instead.
# For array values, use yaml_get_array instead.
#
# $1 yq filter
# $2 yaml file
yaml_get_field() {
  local filter="$1"
  local file="$2"

  returnValue=$("$YQ" -r "$filter" "$file")

  # If the return value was null, we want to return an empty string
  # since it's more inline with bash's behavior.
  if [[ $returnValue == "null" ]]; then
    returnValue=""
  fi

  # Use printf instead of echo to avoid printing a newline
  printf "%s" "$returnValue"
}
