#!/usr/bin/env bats
# Not needed for tests:
# - SC2155: Declare and assign separately to avoid masking return values.
# shellcheck disable=SC2155

load yaml.sh

bats_load_library "bats-support/load.bash"
bats_load_library "bats-assert/load.bash"

setup() {
  YAML_FILE=$(mktemp)
}

teardown() {
  rm -f "$YAML_FILE"
}

@test "yaml_get_array should be able to get values from an array" {
  {
    echo "set:"
    echo "  - bar"
    echo "  - baz"
  } >"$YAML_FILE"

  run yaml_get_array ".set" "$YAML_FILE"
  assert_output "$(echo -e "bar\nbaz")"
}

@test "yaml_get_array should not error if a field is not set" {
  # should not fail
  run yaml_get_array ".not_set" "$YAML_FILE"

  # we expect no output when a field isn't set
  assert_output ""
}

# yaml_get_field tests

@test "yaml_get_field should be able to get a string value" {
  {
    echo "foo: bar"
  } >"$YAML_FILE"

  run yaml_get_field ".foo" "$YAML_FILE"
  assert_output "bar"
}

@test "yaml_get_field should not error if the field is not set" {
  run yaml_get_field ".not_set" "$YAML_FILE"

  # we expect no output when a field isn't set
  assert_output ""
}
