#!/usr/bin/env bash
# Stitch together different manifests to be pulled as one (multi-arch)

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
CI_AUTH_DIR="$DIR/../auth"
LIB_DIR="${DIR}/../../lib"

# shellcheck source=../../lib/bootstrap.sh
source "${LIB_DIR}/bootstrap.sh"

# shellcheck source=../../lib/box.sh
source "${LIB_DIR}/box.sh"

imageRegistry="$(get_box_field 'devenv.imageRegistry')"

# setup docker authentication
if [[ $imageRegistry =~ ^gcr.io/ ]]; then
  # shellcheck source=../auth/gcr.sh
  source "$CI_AUTH_DIR/gcr.sh"
fi

IMAGE_NAME="$imageRegistry/$(get_app_name)"
VERSION="$(make --no-print-directory version)"

archs=(amd64 arm64)
tags=(latest "$VERSION")

for image in /home/circleci/*.tar; do
  echo "Loading docker image: $image"
  docker load -i "$image"
done

if [[ -z $CIRCLE_TAG ]]; then
  echo "Skipping manifest creation, not pushing images ..."
  exit
fi

for tag in "${tags[@]}"; do
  suffixedTags=()
  for arch in "${archs[@]}"; do
    suffixedTags+=("$tag-$arch")
  done

  amendedArgs=()
  for suffixedTag in "${suffixedTags[@]}"; do
    amendedArgs+=("--amend" "$IMAGE_NAME:$suffixedTag")
  done

  for suffixedTag in "${suffixedTags[@]}"; do
    echo "Pushing suffixed tag: $suffixedTag"
    set -x
    docker push "$IMAGE_NAME:$suffixedTag"
    set +x
  done

  echo "Creating Manifest for '$tag' from suffixed tags"
  set -x
  docker manifest create \
    "$IMAGE_NAME:$tag" "${amendedArgs[@]}"
  set +x

  for suffixedTag in "${suffixedTags[@]}"; do
    echo "Pushing Manifest: $tag"
    set -x
    docker manifest push "$IMAGE_NAME:$tag"
    set +x
  done
done
