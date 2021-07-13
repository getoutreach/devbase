#!/usr/bin/env bash
#
# Circle CI generic API-related methods
#

# Helper method to retrieve Circle CI API token, it tries various common env variables that store Circle CI
# API tokens: first CIRCLECI_TOKEN, then CIRCLE_TOKEN and the CIRCLE_API_TOKEN.
# If neither is detected this method exits with an error.
get_circleci_api_token() {
    local token="${CIRCLECI_TOKEN}"
    if [[ -z ${token} ]]; then
        echo "CIRCLECI_TOKEN env value not available, trying CIRCLE_TOKEN"
        token="${CIRCLE_TOKEN}"
        if [[ -z ${token} ]]; then
            echo "CIRCLE_TOKEN env value also not available, trying CIRCLE_API_TOKEN"
            token="${CIRCLE_API_TOKEN}"
            if [[ -z ${token} ]]; then
                echo "CIRCLE_API_TOKEN env value also not available, please set either of the env variables"
                exit 1
            fi
        fi
    fi
    echo "${token}"
}
