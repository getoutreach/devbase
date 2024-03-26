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

imageRegistry="$(get_box_field 'devenv.imageRegistry')"

# setup docker authentication
if [[ $imageRegistry =~ ^gcr.io/ ]]; then
  # shellcheck source=../auth/gcr.sh
  source "$CI_AUTH_DIR/gcr.sh"
fi

APPNAME="$(get_app_name)"
VERSION="$(make --no-print-directory version)"
MANIFEST="$(get_repo_directory)/deployments/docker.yaml"

archs=(amd64 arm64)
tags=(latest "$VERSION")

stitch_and_push_image() {
  local image="$1"
  local remote_image_name
  remote_image_name="$(determine_remote_image_name "$APPNAME" "$imageRegistry" "$image")"

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

    amendedArgs=()
    for suffixedTag in "${suffixedTags[@]}"; do
      amendedArgs+=("--amend" "$remote_image_name:$suffixedTag")
    done

    for suffixedTag in "${suffixedTags[@]}"; do
      echo "Pushing suffixed tag: $suffixedTag"
      run_docker push "$remote_image_name:$suffixedTag"
    done

    echo "Creating Manifest for '$tag' from suffixed tags"
    run_docker manifest create \
      "$remote_image_name:$tag" "${amendedArgs[@]}"

    for suffixedTag in "${suffixedTags[@]}"; do
      echo "Pushing Manifest: $tag"
      run_docker manifest push "$remote_image_name:$tag"
    done
  done
}

# stitch and push all images in the manifest
mapfile -t images < <(docker_manifest_images "$MANIFEST")
for image in "${images[@]}"; do
  stitch_and_push_image "$image"
done
