#!/usr/bin/env bash
# GRPCUI wrapper

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
GOBIN="$DIR/gobin.sh"

# shellcheck source=./lib/bootstrap.sh
source "$DIR/lib/bootstrap.sh"

# We set -plaintext here because we don't use GRPC TLS
args=("-plaintext" "$@")

exec "$GOBIN" "github.com/fullstorydev/grpcui/cmd/grpcui@v$(get_application_version "grpcui")" "${args[@]}"
