#!/usr/bin/env bash
# GRPCUI wrapper

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
GOBIN="$DIR/gobin.sh"

# We set -plaintext here because we don't use GRPC TLS
args=("-plaintext" "$@")

exec "$GOBIN" "github.com/fullstorydev/grpcui/cmd/grpcui@v1.0.0" "${args[@]}"
