#!/usr/bin/env bash
#
# This script uploads Twistlock Image Scan results (in a compact format) to Prisma Cloud.
# This is not a standalone script - it is meant to be called from the upload.sh script only.
#
DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"

IMAGE_SCAN_ARTIFACT_NAME="tmp/test-results/image_scan.json"
SCAN_RESULTS_FILE="/tmp/image_scan.json"
SCAN_RESULTS_OPSLEVEL_FILE="/tmp/image_scan_opslevel.json"
OPSLEVEL_PRISMACI_CUSTOM_EVENT_ID="625cd47f-257a-4ed4-ac56-7efb4dc25158"

ARTIFACTS_JSON_FILE="$1"
ARTIFACT_URL=$(get_artifact_url "${ARTIFACTS_JSON_FILE}" "${IMAGE_SCAN_ARTIFACT_NAME}")

if [[ -z ${ARTIFACT_URL} ]]; then
  echo "Skip OpsLevel Prisma CI upload - could not find an artifact with name ${IMAGE_SCAN_ARTIFACT_NAME}"
  exit 0
fi

# Download the scan results locally
download_artifact_by_full_url "${ARTIFACT_URL}" "${SCAN_RESULTS_FILE}"

# Generate summary file, see image_scan_filter.jq for more details.
jq \
    -f "${DIR}/image_scan_filter.jq" \
    --arg url "${ARTIFACT_URL}" \
    --arg project "${CIRCLE_PROJECT_REPONAME}" \
    "${SCAN_RESULTS_FILE}" \
> "${SCAN_RESULTS_OPSLEVEL_FILE}"

upload_custom_event_to_opslevel "${OPSLEVEL_PRISMACI_CUSTOM_EVENT_ID}" "${SCAN_RESULTS_OPSLEVEL_FILE}"
