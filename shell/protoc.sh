#!/usr/bin/env bash
# This script is a light wrapper around the 'docker-protoc' image (see: https://github.com/namely/docker-protoc),
# which is capable of compiling proto definitions into various languages.
# This is not meant to be used in CircleCI (except by bootstrap itself to validate the general process).
IMAGE="gcr.io/outreach-docker/protoc:1.37_2"
uid=$(id -u)
gid=$(id -g)

SCRIPTS_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"

# shellcheck source=./lib/logging.sh
source "$SCRIPTS_DIR/lib/logging.sh"

# shellcheck source=./lib/bootstrap.sh
source "$SCRIPTS_DIR/lib/bootstrap.sh"

PROTO_DOCS_DIR="$(get_repo_directory)/apidocs/proto"

if [[ -n $CIRCLECI ]]; then
  {
    echo "warning: running protobuf generation in CI is only supported for bootstrap and is DEPRECATED"
    echo "         this will result in only Go protobuf artifacts being generated"
  } >&2
  exec protoc --go_out=plugins=grpc,paths=source_relative:. --proto_path "$(get_repo_directory)/api" "$(get_repo_directory)/api/"*.proto
fi

# Fallback if uid/gid is somehow empty
if [[ -z $uid ]] || [[ -z $gid ]]; then
  echo "Error: Failed to determine the uid/gid of the current user. Defaulting to standard 1000." >&2
  uid=1000
  gid=1000
fi

get_import_basename() {
  local module=$(echo $1 | jq -r .module)
  # sed removes the version off the module path if present
  basename $(go list -f '{{ .Path }}' -m "$module" | sed 's|/v[0-9]||')
}
get_import_path() {
  local module=$(echo $1 | jq -r .module)
  # sed removes the version off the module path if present
  module_root=$(go list -f '{{ .Dir }}' -m "$module" | sed 's|/v[0-9]||')
  echo "${module_root}$(echo $1 | jq -r .path)"
}
for import in $(get_list "go-protoc-imports"); do
  module=$(echo $import | jq -r .module)
  currentver="$(go version | awk '{ print $3 }' | sed 's|go||')"
  requiredver="1.16.0"
  if [ ! "$(printf '%s\n' "$requiredver" "$currentver" | sort -V | head -n1)" = "$requiredver" ]; then
    echo "Go version must be greater than ${requiredver} to use 'go-protoc-imports' feature"
    exit 1
  fi
  go install "$module"
  # check exit for this instead of the install itself, as the install can
  # return non-zero, but still retrieve the dependency. As long as we can
  # retrieve the dep with go list, we can import the files for protoc
  go list -f '{{ .Dir }}' -m "$module"
  exit_code=$?
  if [[ $exit_code != "0" ]]; then
    exit $exit_code
  fi
done
# Create the protoc container.
info "Generating GRPC Clients"
# shellcheck disable=SC2046 # Why: We want it to split
CONTAINER_ID=$(docker run --rm -v "$(get_repo_directory)/api:/defs" \
  $(for import in $(get_list "go-protoc-imports"); do echo "-v $(get_import_path "$import"):/mod/$(get_import_basename "$import")"; done) \
  --entrypoint bash -d "$IMAGE" -c 'exec tail -f /dev/null')

trap 'docker stop -t0 $CONTAINER_ID >/dev/null' EXIT

# Create a localuser matching our gid and uid to prevent issues with file permissions.
docker exec "$CONTAINER_ID" sh -c "groupadd -f --gid $gid localuser && useradd --uid $uid --gid $gid localuser"

# Create the language specific clients
info_sub "go"
# shellcheck disable=SC2046 # Why: We want it to split
docker exec --user localuser "$CONTAINER_ID" entrypoint.sh -f './*.proto' -l go \
  $(for import in $(get_list "go-protoc-imports"); do echo "-i /mod/$(get_import_basename "$import")"; done) \
  $(if has_feature "validation"; then echo "--with-validator --validator-source-relative"; fi) \
  --go-source-relative -o ./ --with-docs html,index.html
mkdir -p "$PROTO_DOCS_DIR"
mv "$(get_repo_directory)"/api/doc/index.html "$PROTO_DOCS_DIR"

if has_grpc_client "node"; then
  info_sub "node"
  # shellcheck disable=SC2046 # Why: We want it to split
  docker exec --user localuser "$CONTAINER_ID" entrypoint.sh -f './*.proto' -l node \
    $(for import in $(get_list "go-protoc-imports"); do echo "-i /mod/$(get_import_basename "$import")"; done) \
    --with-typescript -o "./clients/node/src/grpc/"
fi

if has_grpc_client "ruby"; then
  info_sub "ruby"
  # shellcheck disable=SC2046 # Why: We want it to split
  docker exec --user localuser "$CONTAINER_ID" entrypoint.sh -f './*.proto' -l ruby \
    $(for import in $(get_list "go-protoc-imports"); do echo "-i /mod/$(get_import_basename "$import")"; done) \
    -o "./clients/ruby/lib/$(get_app_name)_client"
fi
