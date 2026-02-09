#!/usr/bin/env bash
# gRPC UI wrapper

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"

# shellcheck source=./lib/mise/stub.sh
source "$DIR/lib/mise/stub.sh"

# We set -plaintext here because we don't use gRPC TLS
args=("-plaintext" "$@")

# check if the grpcui command fails and if so echo error message
if ! mise_exec_tool_with_bin "aqua:fullstorydev/grpcui" grpcui "${args[@]}"; then
  echo >&2
  fatal 'This expects your service to either be running locally or have port forward running.
To port forward:
  - deploy to devenv (i.e. "devenv apps run")
  - run "kubectl port-forward service/[SERVICE-NAME] 5000:5000 -n [NAMESPACE]"'
fi
