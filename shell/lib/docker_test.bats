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

@test "docker_buildx_args defaults" {
  cat >"$YAML_FILE" <<EOF
myservice:
EOF

  MANIFEST="$YAML_FILE" run docker_buildx_args "myservice" "0.10.0" "myservice" "foo/Dockerfile"
  assert_output --partial " --file foo/Dockerfile "
  assert_output --partial " --build-arg VERSION=0.10.0 "
  assert_output --partial " --secret id=npmtoken,env=NPM_TOKEN "
  assert_output --partial " --platform linux/arm64,linux/amd64 "
  refute_output --partial " --tag "
  assert_output --regexp ' \.$'
}

@test "docker_buildx_args uses the specified arch" {
  cat >"$YAML_FILE" <<EOF
myservice:
EOF

  MANIFEST="$YAML_FILE" run docker_buildx_args "myservice" "0.10.0" "myservice" "foo/Dockerfile" "arm64"
  assert_output --partial " --platform linux/arm64 "
}

@test "docker_buildx_args build context with a secondary image" {
  cat >"$YAML_FILE" <<EOF
myservice:
EOF

  MANIFEST="$YAML_FILE" run docker_buildx_args "myservice" "0.10.0" "myimage" "foo/Dockerfile"
  assert_output --regexp '/deployments/myimage$'
}

@test "docker_buildx_args build context specified in manifest with a secondary image" {
  cat >"$YAML_FILE" <<EOF
myservice:
myimage:
  buildContext: foo/bar
EOF

  MANIFEST="$YAML_FILE" run docker_buildx_args "myservice" "0.10.0" "myimage" "foo/Dockerfile"
  assert_output --regexp ' foo/bar$'
}

@test "docker_buildx_args adds tags when CIRCLE_TAG exists" {
  cat >"$YAML_FILE" <<EOF
myservice:
EOF

  CIRCLE_TAG="0.10.0" MANIFEST="$YAML_FILE" run docker_buildx_args "myservice" "0.10.0" "myimage" "foo/Dockerfile"
  assert_output --partial " --tag myimage"
  refute_output --partial " --tag myimage:latest"
}

@test "docker_buildx_args adds tags when CIRCLE_TAG exists and arch specified" {
  cat >"$YAML_FILE" <<EOF
myservice:
EOF

  CIRCLE_TAG="0.10.0" MANIFEST="$YAML_FILE" run docker_buildx_args "myservice" "0.10.0" "myimage" "foo/Dockerfile" "arm64"
  assert_output --partial " --tag myimage"
  assert_output --partial " --tag /myservice/myimage:latest-arm64"
}
