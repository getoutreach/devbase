#!/usr/bin/env bash
# Stitch together different manifests to be pulled as one
# (multi-arch) manifest.
#
# Assumes that shell/ci/release/docker-authn.sh has been run in the same job.

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
LIB_DIR="${DIR}/../../lib"
DOCKER_DIR="${LIB_DIR}/docker/authn"

# shellcheck source=../../lib/bootstrap.sh
source "${LIB_DIR}/bootstrap.sh"

# shellcheck source=../../lib/box.sh
source "${LIB_DIR}/box.sh"

# shellcheck source=../../lib/docker.sh
source "${LIB_DIR}/docker.sh"

# shellcheck source=../../lib/logging.sh
source "${LIB_DIR}/logging.sh"

# shellcheck source=../../lib/docker/authn/aws-ecr.sh
source "${DOCKER_DIR}/aws-ecr.sh"

imageRegistries="$(get_docker_push_registries)"

APPNAME="$(get_app_name)"
VERSION="$(get_app_version)"
MANIFEST="$(get_repo_directory)/deployments/docker.yaml"

if [[ -n ${CUSTOM_ARCHES:-} ]]; then
  info "Using arches from environment"
  IFS=' ' read -r -a archs <<<"$CUSTOM_ARCHES"
else
  archs=(amd64 arm64)
fi

info "Arches to stitch together: ${archs[*]}"

tags=(latest "$VERSION")
will_push="$(will_push_images)"

stitch_and_push_image() {
  local image="$1"
  local remoteImageNames=()
  for imageRegistry in $imageRegistries; do
    local remoteImageName
    remoteImageName="$(determine_remote_image_name "$APPNAME" "$imageRegistry" "$image")"
    remoteImageNames+=("$remoteImageName")
    if [[ $will_push == "true" ]] && [[ $remoteImageName =~ amazonaws.com($|/) ]]; then
      ensure_ecr_repository "$remoteImageName"
    fi
  done

  for img_filename in /home/circleci/"$image"-*.tar; do
    echo "Loading docker image: $img_filename"
    run_docker load -i "$img_filename"
  done

  if [[ $will_push == "false" ]]; then
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
