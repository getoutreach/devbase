#!/usr/bin/env bash
# Stitch together different manifests to be pulled as one
# (multi-arch) manifest.

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
CI_AUTH_DIR="$DIR/../auth"
LIB_DIR="${DIR}/../../lib"

# shellcheck source=../../lib/bootstrap.sh
source "${LIB_DIR}/bootstrap.sh"

# shellcheck source=../../lib/box.sh
source "${LIB_DIR}/box.sh"

# shellcheck source=../../lib/docker.sh
source "${LIB_DIR}/docker.sh"

imageRegistries="${DOCKER_PUSH_REGISTRIES:-$(get_box_field 'docker.imagePushRegistries')}"
if [[ -z $imageRegistries ]]; then
  # Fall back to the old box field
  imageRegistries="$(get_box_field 'devenv.imageRegistry')"
fi

# setup docker authentication
if [[ $imageRegistries =~ gcr.io/ ]]; then
  # shellcheck source=../auth/gcr.sh
  source "$CI_AUTH_DIR/gcr.sh"
fi

if [[ $imageRegistries =~ amazonaws.com/ ]]; then
  # The auth script uses $DOCKER_PUSH_REGISTRIES to determine which registries to authenticate.
  DOCKER_PUSH_REGISTRIES="$imageRegistries"
  # shellcheck source=../auth/aws-ecr.sh
  source "$CI_AUTH_DIR/aws-ecr.sh"
fi

APPNAME="$(get_app_name)"
VERSION="$(make --no-print-directory version)"
MANIFEST="$(get_repo_directory)/deployments/docker.yaml"

archs=(amd64 arm64)
tags=(latest "$VERSION")

stitch_and_push_image() {
  local image="$1"
  local remoteImageNames=()
  for imageRegistry in $imageRegistries; do
    remoteImageNames+=("$(determine_remote_image_name "$APPNAME" "$imageRegistry" "$image")")
  done

  for img_filename in /home/circleci/"$image"-*.tar; do
    echo "Loading docker image: $img_filename"
    run_docker load -i "$img_filename"
  done

  if [[ -z $CIRCLE_TAG ]]; then
    echo "Skipping manifest creation, not pushing images ..."
    return
  fi

  for tag in "${tags[@]}"; do
    suffixedTags=()
    for arch in "${archs[@]}"; do
      suffixedTags+=("$tag-$arch")
    done

    for remoteImageName in "${remoteImageNames[@]}"; do
      amendedArgs=()
      for suffixedTag in "${suffixedTags[@]}"; do
        amendedArgs+=("--amend" "$remoteImageName:$suffixedTag")
      done

      for suffixedTag in "${suffixedTags[@]}"; do
        echo "Pushing suffixed tag: $suffixedTag"
        run_docker push "$remoteImageName:$suffixedTag"
      done

      echo "Creating Manifest for '$tag' from suffixed tags"
      run_docker manifest create \
        "$remoteImageName:$tag" "${amendedArgs[@]}"

      for suffixedTag in "${suffixedTags[@]}"; do
        echo "Pushing Manifest: $tag"
        run_docker manifest push "$remoteImageName:$tag"
      done
    done
  done
}

# stitch and push all images in the manifest
mapfile -t images < <(docker_manifest_images "$MANIFEST")
for image in "${images[@]}"; do
  stitch_and_push_image "$image"
done
