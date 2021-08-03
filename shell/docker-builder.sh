#!/usr/bin/env bash
# Builds a docker image in CircleCI
set -e

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
LIB_DIR="${DIR}/lib"
SEC_DIR="${DIR}/security"
TWIST_SCAN_DIR="${SEC_DIR}/prismaci"
VERSION="$(make version)"

# shellcheck source=./lib/bootstrap.sh
source "${DIR}/lib/bootstrap.sh"

appName="$(get_app_name)"
remote_image_name="gcr.io/outreach-docker/${appName}"

# setup docker authentication
# shellcheck source=./lib/docker-authn.sh
source "${LIB_DIR}/docker-authn.sh"

# shellcheck source=./lib/buildx.sh
source "${LIB_DIR}/buildx.sh"

# shellcheck source=./lib/ssh-auth.sh
source "${LIB_DIR}/ssh-auth.sh"

args=("--ssh" "default" "--progress=plain" "--file" "deployments/${appName}/Dockerfile" "--build-arg" "VERSION=${VERSION}")

# Build a quick native image on PRs and load it into docker cache
# for security scanning
if [[ -z $CIRCLE_TAG ]]; then
  info "Building Docker Image (test)"
  docker buildx build "${args[@]}" -t "${appName}" --load .

  info "🔐 Scanning docker image for vulnerabilities"
  "${TWIST_SCAN_DIR}/twist-scan.sh" "${appName}" || echo "Warning: Failed to scan image"
fi

if [[ -n $CIRCLE_TAG ]]; then
  echo "🔨 Building and Pushing Docker Image (production)"
  set -x
  docker buildx build "${args[@]}" --platform linux/arm64,linux/amd64 \
    -t "${remote_image_name}:${VERSION}" -t "$remote_image_name:latest" --push .
  set +x
fi
