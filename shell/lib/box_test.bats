#!/usr/bin/env bats

load box.sh

bats_load_library "bats-support/load.bash"
bats_load_library "bats-assert/load.bash"

setup() {
  # This points us to use a temp file for the box configuration as
  # opposed to the real one. This prevents local box configuration from
  # mutating the test results.
  BOXPATH=$(mktemp)
}

teardown() {
  rm -rf "$BOXPATH"
}

@test "get_box_field should return a value if it exists" {
  {
    echo "config:"
    echo "  foo: bar"
  } >"$BOXPATH"

  run get_box_field "foo"
  assert_output "bar"
}

@test "get_box_field should return no data if a field doesn't exist" {
  run get_box_field "baz"
  assert_output ""
}

@test "get_box_field should support nested fields" {
  {
    echo "config:"
    echo "  foo:"
    echo "    bar: baz"
  } >"$BOXPATH"

  run get_box_field "foo.bar"
  assert_output "baz"
}

@test "get_box_yaml should return the contents of the box file" {
  {
    echo "config:"
    echo "  foo: bar"
  } >"$BOXPATH"

  run get_box_yaml
  assert_output "$(cat "$BOXPATH")"
}

@test "get_box_yaml should fail if cloning the box repo fails" {
  BOX_REPOSITORY_URL="https://github.com/getoutreach/notaboxrepo" \
    BOXPATH=/foo/bar/nonexistent \
    run get_box_yaml
  assert_failure
  assert_line "Cloning failed, cannot find box.yaml"
}
