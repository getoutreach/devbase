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

# setup docker authentication
# shellcheck source=./lib/docker-authn.sh
source "$LIB_DIR/docker-authn.sh"

# shellcheck source=./lib/buildx.sh
source "$LIB_DIR/buildx.sh"

# shellcheck source=./lib/ssh-auth.sh
source "$LIB_DIR/ssh-auth.sh"

args=("--ssh" "default" "--progress=plain" "--file" "deployments/$appName/Dockerfile" "--build-arg" "VERSION=${VERSION}")
if [[ -n $CIRCLE_TAG ]]; then
  # Only push on a tag
  extraArgs+=("--push")
fi

# Build a quick native image on PRs and load it into docker cache
# for security scanning
if [[ -e "/usr/local/bin/twist-scan.sh" ]] && [[ -z $CIRCLE_TAG ]]; then
  info "Building Docker Image (test)"
  docker buildx build "${args[@]}" -t "$appName" --load .

  info "🔐 Scanning docker image for vulnerabilities"
  /usr/local/bin/twist-scan.sh "$appName"
fi

if [[ -n $CIRCLE_TAG ]]; then
  echo "🔨 Building and Pushing Docker Image (production)"
  set -x
  docker buildx build "${args[@]}" --platform linux/arm64,linux/amd64 \
    -t "$remote_image_name:$VERSION" --push .
  set +x
fi
