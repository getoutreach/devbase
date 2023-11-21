#!/usr/bin/env bash

bats_load_library "bats-support/load.bash"
bats_load_library "bats-assert/load.bash"

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

@test "determine_remote_image_name with pushTo set in the manifest" {
  cat >"$YAML_FILE" <<EOF
myservice:
  pushTo: example.com/pushTo/something
EOF

  MANIFEST="$YAML_FILE" run determine_remote_image_name "myservice" "example.com/registry" "myservice"
  assert_output "example.com/pushTo/something"
}

@test "determine_remote_image_name with the main image" {
  cat >"$YAML_FILE" <<EOF
myservice:
EOF

  MANIFEST="$YAML_FILE" run determine_remote_image_name "myservice" "example.com/registry" "myservice"
  assert_output "example.com/registry/myservice"
}

@test "determine_remote_image_name with a secondary image" {
  cat >"$YAML_FILE" <<EOF
myservice:
EOF

  MANIFEST="$YAML_FILE" run determine_remote_image_name "myservice" "example.com/registry" "secondary"
  assert_output "example.com/registry/myservice/secondary"
}
