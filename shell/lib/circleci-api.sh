#!/usr/bin/env bash
#
# Circle CI generic API-related methods
#

# Helper method to retrieve Circle CI API token, it tries various common env variables that store Circle CI
# API tokens: first CIRCLECI_TOKEN, then CIRCLE_TOKEN and the honeycomb's BUILDEVENT_CIRCLE_API_TOKEN.
# If neither is detected this method exits with an error.
get_circleci_api_token() {
    local token="${CIRCLECI_TOKEN}"
    if [[ -z ${token} ]]; then
        echo "CIRCLECI_TOKEN env value not available, trying CIRCLE_TOKEN"
        token="${CIRCLE_TOKEN}"
        if [[ -z ${token} ]]; then
            echo "CIRCLE_TOKEN env value also not available, trying honeycombio's BUILDEVENT_CIRCLE_API_TOKEN"
            token="${BUILDEVENT_CIRCLE_API_TOKEN}"
            if [[ -z ${token} ]]; then
                echo "BUILDEVENT_CIRCLE_API_TOKEN env value also not available, please set either CIRCLECI_TOKEN or CIRCLE_TOKEN"
                exit 1
            fi
        fi
    fi
    echo "${token}"
}
