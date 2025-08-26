#!/usr/bin/env bash
# GRPCUI wrapper

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"

# shellcheck source=./lib/bootstrap.sh
source "$DIR/lib/bootstrap.sh"

# shellcheck source=./lib/logging.sh
source "$DIR/lib/logging.sh"

# shellcheck source=./lib/mise.sh
source "$DIR/lib/mise.sh"

# We set -plaintext here because we don't use gRPC TLS
args=("-plaintext" "$@")

# check if the grpcui command fails and if so echo error message
if ! mise_exec "aqua:fullstorydev/grpcui@v$(get_tool_version "grpcui")" grpcui "${args[@]}"; then
  echo >&2
  error 'this expects your service to either be running locally or have port forward running.
to port forward:
  - deploy to devenv (i.e. "devenv app deploy .")
  - run "kubectl port-forward service/[SERVICE-NAME] 5000:5000 -n [NAMESPACE]"'
fi
