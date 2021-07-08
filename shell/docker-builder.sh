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

extraArgs=("-t" "$remote_image_name:${VERSION}")
if [[ -n ${CIRCLE_TAG} ]]; then
  # Only push on a tag
  extraArgs+=("--push")
else
  echo "Note: Skipping twist-scan due to buildx limitations"
  # TODO: buildx doesn't currently support this.
  # Load it into the docker cache so we can run twist-scan
  #extraArgs+=("--load")
fi

echo "🔨 Building Docker Image"
set -x
docker buildx build --ssh default --progress=plain \
  --platform linux/arm64,linux/amd64 \
  --file "deployments/${appName}/Dockerfile" \
  --build-arg "VERSION=${VERSION}" "${extraArgs[@]}" .
set +x

if [[ -z ${CIRCLE_TAG} ]]; then
  # Scan the built image
  info "Scanning docker image for vulnerabilities"
  source "${TWIST_SCAN_DIR}/twist-scan.sh" "${appName}"
fi
