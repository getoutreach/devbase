#!/usr/bin/env bash

bats_load_library "bats-support/load.bash"
bats_load_library "bats-assert/load.bash"

load docker.sh

setup() {
  YAML_FILE=$(mktemp)
  BOXPATH=$(mktemp)
}

teardown() {
  rm -f "$YAML_FILE"
  rm -f "$BOXPATH"
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
  cat >"$BOXPATH" <<EOF
config:
  docker:
    imagePushRegistries:
      - example.com
EOF

  cat >"$YAML_FILE" <<EOF
myservice:
EOF

  CIRCLE_TAG="0.10.0" MANIFEST="$YAML_FILE" run docker_buildx_args "myservice" "0.10.0" "myimage" "foo/Dockerfile" "arm64"
  assert_output --partial " --tag myimage"
  assert_output --partial " --tag example.com/myservice/myimage:latest-arm64"
}

@test "docker_buildx_args with BOX_DOCKER_PUSH_IMAGE_REGISTRIES" {
  cat >"$BOXPATH" <<EOF
config:
  docker:
    imagePushRegistries:
      - example.com
EOF
  cat >"$YAML_FILE" <<EOF
myservice:
EOF

  BOX_DOCKER_PUSH_IMAGE_REGISTRIES="example.com/box1 example.com/box2" \
    CIRCLE_TAG="0.10.0" \
    MANIFEST="$YAML_FILE" \
    run docker_buildx_args "myservice" "0.10.0" "myimage" "foo/Dockerfile" "arm64"

  assert_output --partial " --tag example.com/box1/myservice/myimage:latest-arm64"
  assert_output --partial " --tag example.com/box1/myservice/myimage:0.10.0-arm64"
  assert_output --partial " --tag example.com/box2/myservice/myimage:latest-arm64"
  assert_output --partial " --tag example.com/box2/myservice/myimage:0.10.0-arm64"
}

@test "get_docker_push_registries from box config" {
  cat >"$BOXPATH" <<EOF
config:
  docker:
    imagePushRegistries:
      - example.com
      - example.org
EOF

  run get_docker_push_registries
  assert_output <<EOF
example.com
example.org
EOF
}

@test "get_docker_push_registries from DOCKER_PUSH_REGISTRIES" {
  cat >"$BOXPATH" <<EOF
config:
  docker:
    imagePushRegistries:
      - example.com
      - example.org
EOF

  DOCKER_PUSH_REGISTRIES="example.com/docker-push-registries" run get_docker_push_registries
  assert_output example.com/docker-push-registries
}

@test "get_docker_push_registries from BOX_DOCKER_PUSH_IMAGE_REGISTRIES" {
  cat >"$BOXPATH" <<EOF
config:
  docker:
    imagePushRegistries:
      - example.com
      - example.org
EOF

  BOX_DOCKER_PUSH_IMAGE_REGISTRIES="example.com/box1 example.com/box2" \
    DOCKER_PUSH_REGISTRIES="example.com/docker-push-registries" \
    run get_docker_push_registries
  assert_output "example.com/box1 example.com/box2"
}

@test "get_docker_pull_registry prefers BOX_DOCKER_PULL_REGISTRY" {
  cat >"$BOXPATH" <<EOF
config:
  docker:
    imagePullRegistry: docker.imagepullregistry.example
  devenv:
    imageRegistry: devenv.imageregistry.example
EOF

  BOX_DOCKER_PULL_IMAGE_REGISTRY="box.env.example" run get_docker_pull_registry
  assert_output "box.env.example"
}

@test "get_docker_pull_registry prefers docker.imagePullRegistry over devenv.imageRegistry" {
  cat >"$BOXPATH" <<EOF
config:
  docker:
    imagePullRegistry: docker.imagepullregistry.example
  devenv:
    imageRegistry: devenv.imageregistry.example
EOF

  run get_docker_pull_registry
  assert_output "docker.imagepullregistry.example"
}

@test "get_docker_pull_registry falls back to devenv.imageRegistry" {
  cat >"$BOXPATH" <<EOF
config:
  devenv:
    imageRegistry: devenv.imageregistry.example
EOF

  run get_docker_pull_registry
  assert_output "devenv.imageregistry.example"
}

@test "will_push_image with no related variables set returns false" {
  run will_push_images
  assert_output "false"
}

@test "will_push_image with only CIRCLE_TAG set returns true" {
  CIRCLE_TAG="abcd" run will_push_images
  assert_output "true"
}

@test "will_push_image with CIRCLE_TAG set and VERSIONING_SCHEME=semver returns true" {
  CIRCLE_TAG="abcd" VERSIONING_SCHEME="semver" run will_push_images
  assert_output "true"
}

@test "will_push_image with CIRCLE_TAG set and VERSIONING_SCHEME=sha returns false" {
  CIRCLE_TAG="abcd" VERSIONING_SCHEME="sha" run will_push_images
  assert_output "true"
}

@test "will_push_image with only VERSIONING_SCHEME=sha returns true" {
  VERSIONING_SCHEME="sha" run will_push_images
  assert_output "true"
}

@test "will_push_image with VERSIONING_SCHEME=sha and DRY_RUN=true returns false" {
  VERSIONING_SCHEME="sha" DRY_RUN="true" run will_push_images
  assert_output "false"
}
