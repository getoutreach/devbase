#!/usr/bin/env bash
# Tests docker.sh functions.

# No -e because we want to handle errors to make them
# obvious.
set -uo pipefail

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"

export TESTING_DO_NOT_BUILD=1
# shellcheck source=docker.sh
source "${DIR}/docker.sh"

test_get_image_field() {
  local testdata="$DIR/testdata"

  echo "Should be able to get a string value"
  buildContextTest=$(get_image_field "default" "buildContext" "string" "$testdata/test.yaml")
  if [[ $buildContextTest != "helloWorld" ]]; then
    echo "Expected buildContext to be 'helloWorld', got '$buildContextTest'" >&2
    exit 1
  fi

  echo "Should be able to get an array value"
  mapfile -t secretsTest < <(get_image_field "default" "secrets" "array" "$testdata/test.yaml")
  if [[ ${secretsTest[*]} != "hello world" ]]; then
    echo "Unexpected secrets value, got '${secretsTest[*]}'" >&2
    exit 1
  fi
}

test_get_image_field
