#!/usr/bin/env bash
# GRPCUI wrapper

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
GOBIN="$DIR/gobin.sh"

# shellcheck source=./lib/bootstrap.sh
source "$DIR/lib/bootstrap.sh"

# We set -plaintext here because we don't use GRPC TLS
args=("-plaintext" "$@")

# check if the grpcui command failes and if so echo error message
if ! "$GOBIN" "github.com/fullstorydev/grpcui/cmd/grpcui@v$(get_application_version "grpcui")" "${args[@]}"; then
  echo
  echo 'this expects your service to either be running locally or have port forward running.
to port forward:
  - deploy to devenv (i.e. "devenv app deploy .")
  - run "kubectl port-forward service/[SERVICE-NAME] 5000:5000 -n [NAMESPACE]"'
fi
