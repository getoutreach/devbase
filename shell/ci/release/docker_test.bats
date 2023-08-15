#!/usr/bin/env bash

bats_load_library "bats-support/load.bash"
bats_load_library "bats-assert/load.bash"

# This ensures that we don't do the build logic. Eventually this will be
# removed in favor of testing real functions. Must come before the "load
# docker.sh" statement.
export TESTING_DO_NOT_BUILD=1
load docker.sh

setup() {
	YAML_FILE=$(mktemp)
}

teardown() {
	rm -f "$YAML_FILE"
}

@test "get_image_field should be able to get a string value" {
	cat >"$YAML_FILE" <<EOF
default:
  buildContext: helloWorld
EOF

	run get_image_field "default" "buildContext" "string" "$YAML_FILE"
	assert_output "helloWorld"
}

@test "get_image_field should be able to get an array value" {
	cat >"$YAML_FILE" <<EOF
default:
  secrets:
  - hello
  - world
EOF

	run get_image_field "default" "secrets" "array" "$YAML_FILE"
	assert_output "$(echo -e "hello\nworld")"
}
