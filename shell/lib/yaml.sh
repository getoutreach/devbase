#!/usr/bin/env bash
# yaml is a general purpose bash yaml parsing library

# yaml_get_array returns a newline separated list of values
# from a yaml array. If a value is not set, it will return
# an empty string.
#
# $1 yq filter
# $2 yaml file
yaml_get_array() {
  local filter="$1"
  local file="$2"
  yq -r "$filter | .[]?" "$file"
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
