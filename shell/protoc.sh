#!/usr/bin/env bash
# This script is a light wrapper around the 'docker-protoc' image (see: https://github.com/namely/docker-protoc),
# which is capable of compiling proto definitions into various languages.
# This is not meant to be used in CircleCI (except by bootstrap itself to validate the general process).
IMAGE="gcr.io/outreach-docker/protoc:latest"
uid=$(id -u)
gid=$(id -g)

SCRIPTS_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"

# shellcheck source=./lib/logging.sh
source "$SCRIPTS_DIR/lib/logging.sh"

# shellcheck source=./lib/bootstrap.sh
source "$SCRIPTS_DIR/lib/bootstrap.sh"

if [[ -n $CIRCLECI ]]; then
  {
    echo "warning: running protobuf generatation in CI is only supported for bootstrap and is DEPRECATED"
    echo "         this will result in only Go protobuf artifacts being generated"
  } >&2
  exec protoc -I. --go_out=plugins=grpc,paths=source_relative:. "$(get_repo_directory)/api/*.proto"
fi

# Fallback if uid/gid is somehow empty
if [[ -z $uid ]] || [[ -z $gid ]]; then
  echo "Error: Failed to determine the uid/gid of the current user. Defaulting to standard 1000." >&2
  uid=1000
  gid=1000
fi

# Create the protoc container.
info "Generating GRPC Clients"
CONTAINER_ID=$(docker run --rm -v "$(get_repo_directory)/api:/defs" \
  --entrypoint bash -d "$IMAGE" -c 'exec tail -f /dev/null')

trap 'docker stop -t0 $CONTAINER_ID >/dev/null' EXIT

# Create a localuser matching our gid and uid to prevent issues with file permissions.
docker exec "$CONTAINER_ID" sh -c "groupadd -f --gid $gid localuser && useradd --uid $uid --gid $gid localuser"

# Create the language specific clients
info_sub "go"
docker exec --user localuser "$CONTAINER_ID" entrypoint.sh -f './*.proto' -l go \
  --go-source-relative -o ./

if has_grpc_client "node"; then
  info_sub "node"
  docker exec --user localuser "$CONTAINER_ID" entrypoint.sh -f './*.proto' -l node \
    --with-typescript -o "./clients/node/src/grpc/"
fi

if has_grpc_client "ruby"; then
  info_sub "ruby"
  docker exec --user localuser "$CONTAINER_ID" entrypoint.sh -f './*.proto' -l ruby \
    -o "./clients/ruby/lib/$(get_app_name)_client"
fi
