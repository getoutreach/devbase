#!/usr/bin/env bash
# OpsLevel methods

# Uploads a custom event to the webhook of the OpsLevel integration.
# See upload_prismaci.sh for usage sample.
upload_custom_event_to_opslevel() {
    local event_id="$1"
    local event_json_file="$2"

    curl \
        -X POST "https://app.opslevel.com/integrations/custom_event/${event_id}" \
        -H 'content-type: application/json' \
        --data-binary "@${event_json_file}"
}
