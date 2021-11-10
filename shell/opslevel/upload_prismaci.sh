#!/usr/bin/env bash
#
# This script uploads Twistlock Image Scan results (in a compact format) to Prisma Cloud.
# This is not a standalone script - it is meant to be called from the upload.sh script only.
#
DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"

IMAGE_SCAN_ARTIFACT_NAME="tmp/test-results/image_scan.json"
SCAN_RESULTS_FILE="/tmp/image_scan.json"
SCAN_RESULTS_OPSLEVEL_FILE="/tmp/image_scan_opslevel.json"

if [[ -z $OL_PC_WEBHOOK_URL ]]; then
  echo "To enable Prisma CI scan summary upload to OpsLevel, set OL_PC_WEBHOOK_URL to the integration web hook URL"
  exit 0
fi

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
  >"${SCAN_RESULTS_OPSLEVEL_FILE}"

# log the summary for troubleshooting, it is short
echo "Image Scan summary to be shared with OpsLevel:"
cat "${SCAN_RESULTS_OPSLEVEL_FILE}"
echo "Uploading the summary to ${OL_PC_WEBHOOK_URL}:"
upload_custom_event_to_opslevel "${OL_PC_WEBHOOK_URL}" "${SCAN_RESULTS_OPSLEVEL_FILE}"
echo ""
echo "Prisma CI scan summary has been uploaded to OpsLevel!"
