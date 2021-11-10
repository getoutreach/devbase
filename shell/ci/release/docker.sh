#!/usr/bin/env bash
# Builds a docker image, and pushes it if it's in CircleCI
set -e

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
LIB_DIR="${DIR}/../../lib"
SEC_DIR="${DIR}/../../security"
TWIST_SCAN_DIR="${SEC_DIR}/../../prismaci"
VERSION="$(make version)"

# shellcheck source=../../lib/bootstrap.sh
source "${LIB_DIR}/bootstrap.sh"

appName="$(get_app_name)"
remote_image_name="gcr.io/outreach-docker/${appName}"

# shellcheck source=../../lib/buildx.sh
source "${LIB_DIR}/buildx.sh"

# shellcheck source=../../lib/logging.sh
source "${LIB_DIR}/logging.sh"

cache_dir="$HOME/.cache/docker-layers"

secrets=("--secret" "id=npmtoken,env=NPM_TOKEN")

args=(
  "--ssh" "default"
  "--progress=plain" "--file" "deployments/${appName}/Dockerfile"
  "--build-arg" "VERSION=${VERSION}"
  "--cache-from" "type=local,src=${cache_dir}"
  "--cache-to" "type=local,dest=${cache_dir}"
)

# Build a quick native image and load it into docker cache for security scanning
# Scan reports for release images are also uploaded to OpsLevel (test image reports only available on PR runs as artifacts).
info "Building Docker Image (for scanning)"
docker buildx build "${args[@]}" "${secrets[@]}" -t "${appName}" --load .

info "🔐 Scanning docker image for vulnerabilities"
"${TWIST_SCAN_DIR}/twist-scan.sh" "${appName}" || echo "Warning: Failed to scan image"

if [[ -n $CIRCLE_TAG ]]; then
  echo "🔨 Building and Pushing Docker Image (production)"
  set -x
  docker buildx build "${args[@]}" "${secrets[@]}" --platform linux/arm64,linux/amd64 \
    -t "${remote_image_name}:${VERSION}" -t "$remote_image_name:latest" --push .
  set +x
fi
