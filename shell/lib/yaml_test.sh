#!/usr/bin/env bash
# Tests the shell/lib/yaml.sh library
#
# Not needed for tests:
# - SC2155: Declare and assign separately to avoid masking return values.
# shellcheck disable=SC2155

# No -e because we want to handle errors to make them
# obvious.
set -uo pipefail

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"

# shellcheck source=../lib/yaml.sh
source "${DIR}/../lib/yaml.sh"

test_yaml_get_array() {
  # should be able to get values from an array
  local yaml_file=$(mktemp)
  {
    echo "set:"
    echo "  - bar"
    echo "  - baz"
  } >"$yaml_file"

  echo "Should be able to get values from an array"
  local got=$(yaml_get_array ".set" "$yaml_file")
  local expected=$({
    echo "bar"
    echo "baz"
  })
  if [[ $got != "$expected" ]]; then
    echo "Expected '$expected', got '$got'"
    exit 1
  fi

  echo "Should not error if the field is not set"
  got=$(yaml_get_array ".not_set" "$yaml_file" 2>&1)
  if [[ $got != "" ]]; then
    echo "Expected empty value for unset field, got: $got" >&2
    exit 1
  fi

  rm -f "$yaml_file"
  return 0
}

test_yaml_get_array
