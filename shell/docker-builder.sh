#!/usr/bin/env bash
# DEPRECATED: Use below path instead.
# Builds a docker image in CircleCI
set -e

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
exec "$DIR/ci/release/docker.sh"
