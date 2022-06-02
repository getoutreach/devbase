#!/usr/bin/env bash
# Configures CircleCI docker authentication
set -e

if [[ -z $GCLOUD_SERVICE_ACCOUNT ]]; then
  echo "Skipped: GCLOUD_SERVICE_ACCOUNT is not set."
  exit 0
fi

docker login \
  -u _json_key \
  --password-stdin \
  https://gcr.io <<<"${GCLOUD_SERVICE_ACCOUNT}"

# Key needs to be written to a file to authorize gcloud CLI.
echo "$GCLOUD_SERVICE_ACCOUNT" >gcloud-auth-key.json

gcloud auth activate-service-account \
  circleci-rw@outreach-docker.iam.gserviceaccount.com \
  --key-file=gcloud-auth-key.json

rm -f gcloud-auth-key.json
