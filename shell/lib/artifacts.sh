#!/usr/bin/env bash
# Circle CI Artifact methods

# Downloads artifacts metadata from the circle CI (in JSON format).
download_artifacts_json() {
  local target_file="$1"
  ARTIFACTS_API_URL="https://circleci.com/api/v1.1/project/github/${CIRCLE_PROJECT_USERNAME}/${CIRCLE_PROJECT_REPONAME}/${CIRCLE_BUILD_NUM}/artifacts"
  echo "Downloading artifacts from ${ARTIFACTS_API_URL}:"
  ARTIFACTS_JSON=$(curl -H "Circle-Token: ${CIRCLE_API_TOKEN}" "${ARTIFACTS_API_URL}")
  echo "${ARTIFACTS_JSON}" >"${target_file}"
  # also echo the artifacts content to console/log for troubleshooting
  echo "${ARTIFACTS_JSON}"
}

# extracts specific artifact URL from the artifacts metadata (JSON format)
get_artifact_url() {
  local artifacts_json_file="$1"
  local path="$2"

  ARTIFACT_URL=$(jq -r '.[] | select(.path=="'"${path}"'") | .url' "${artifacts_json_file}")
  echo "${ARTIFACT_URL}"
}

# given the full URL of the artifact, download it to the local file (second param)
download_artifact_by_full_url() {
  local full_url="$1"
  local local_file="$2"

  echo "Downloading ${full_url}"
  curl -L -H "Circle-Token: ${CIRCLE_API_TOKEN}" "${full_url}" >"${local_file}"
}
