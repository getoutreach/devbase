#!/usr/bin/env bash
# OpsLevel methods

# Uploads a custom event to the webhook of the OpsLevel integration.
# See upload_prismaci.sh for usage sample.
upload_custom_event_to_opslevel() {
  local webhook_url="$1"
  local event_json_file="$2"

  curl \
    -X POST "${webhook_url}" \
    -H 'content-type: application/json' \
    --data-binary "@${event_json_file}"
}
