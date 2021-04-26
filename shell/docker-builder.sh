#!/usr/bin/env bash
# Builds a docker image in CircleCI
set -e

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
LIB_DIR="$DIR/lib"
VERSION="$(make version)"

# shellcheck source=./lib/bootstrap.sh
source "$DIR/lib/bootstrap.sh"

appName="$(get_app_name)"
remote_image_name="gcr.io/outreach-docker/$appName"

# shellcheck source=./lib/logging.sh
source "$LIB_DIR/logging.sh"

# setup docker authentication
# shellcheck source=./lib/docker-authn.sh
source "$LIB_DIR/docker-authn.sh"

info "setting up ssh access"
# Setup SSH access
eval "$(ssh-agent)"
ssh-add -D

# HACK: This is a fragile attempt to add whatever key is for github.com to our ssh-agent
grep -A 2 github.com ~/.ssh/config | grep IdentityFile | awk '{ print $2 }' | xargs -n 1 ssh-add

info "building docker image"
DOCKER_BUILDKIT=1 docker build --ssh default --progress=plain \
  -t "$appName" \
  --file "deployments/$appName/Dockerfile" \
  --build-arg "VERSION=${VERSION}" \
  .

# Scan the built image 
info "Scanning docker image for vulnerabilities"
/usr/local/bin/twist-scan.sh "$appName"

declare -a TAGS
# Handle released versions
if [[ -n $CIRCLE_TAG ]]; then
  # Note: $VERSION is needed for Maestro when using the semver strategy
  TAGS+=("$VERSION" "latest")
else # Handle non-release/main branches here, we only tag based on a calculated branch tag
  # strip the branch name of invalid spec characters
  TAGS+=("$VERSION-branch.${CIRCLE_BRANCH//[^a-zA-Z\-\.]/-}")
fi

for tag in "${TAGS[@]}"; do
  # fqin is the fully-qualified image name, it's tag is truncated to 127 characters to match the
  # docker tag length spec: https://docs.docker.com/engine/reference/commandline/tag/
  fqin="$remote_image_name:$(cut -c 1-127 <<<"$tag")"

  info "pushing image '$fqin'"
  docker tag "$appName" "$fqin"
  docker push "$fqin"
done
