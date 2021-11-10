#!/usr/bin/env bash
# Configures CircleCI docker authentication
set -e

docker login \
  -u _json_key \
  --password-stdin \
  https://gcr.io <<<"${GCLOUD_SERVICE_ACCOUNT}"
