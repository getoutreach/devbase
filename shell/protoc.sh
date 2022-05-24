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

# Fallback if uid/gid is somehow empty
if [[ -z $uid ]] || [[ -z $gid ]]; then
  echo "Error: Failed to determine the uid/gid of the current user. Defaulting to standard 1000." >&2
  uid=1000
  gid=1000
fi

get_import_basename() {
  # shellcheck disable=SC2155 # Why: splitting declartion is messier
  local module="$(jq -r .module <<<"$1")"
  # sed removes the version off the module path if present
  basename "$(go list -f '{{ .Path }}' -m "$module" | sed 's|/v[0-9]||')"
}
get_import_path() {
  # shellcheck disable=SC2155 # Why: splitting declartion is messier
  local module="$(jq -r .module <<<"$1")"
  # sed removes the version off the module path if present
  module_root="$(go list -f '{{ .Dir }}' -m "$module" | sed 's|/v[0-9]||')"
  echo "${module_root}$(jq -r .path <<<"$1")"
}
for import in $(get_list "go-protoc-imports"); do
  module=$(jq -r .module <<<"$import")
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

# This is where the plugins will be installed, the path could definitely change
# since we're using asdf to manage go versions.
goenvbin="$(go env GOPATH)/bin"

info "Ensuring protoc plugins are installed"

info_sub "protoc-gen-validate"
go get -d github.com/envoyproxy/protoc-gen-validate@v0.6.7
go install github.com/envoyproxy/protoc-gen-validate@v0.6.7

info_sub "protoc-gen-go"
go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.28

info_sub "protoc-gen-go-grpc"
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.2

info_sub "protoc-gen-doc"
go install github.com/pseudomuto/protoc-gen-doc/cmd/protoc-gen-doc@v1.5.1

info "Generating GRPC Clients"

# Create the language specific clients
info_sub "go"
exec protoc \
  --plugin=protoc-gen-go=$goenvbin/protoc-gen-go --plugin=protoc-gen-go-grpc=$goenvbin/protoc-gen-go-grpc \
  --go_out=. --go_opt=paths=source_relative \
  --go-grpc_out=. --go-grpc_opt=paths=source_relative \
  $(for import in $(get_list "go-protoc-imports"); do echo "--proto_path=/mod/$(get_import_basename "$import")"; done) \
  $(if has_feature "validation"; then echo "--proto_path=$(go env GOPATH)/src/github.com/envoyproxy/protoc-gen-validate --plugin=protoc-gen-validate=$goenvbin/protoc-gen-validate --validate_out=lang=go:$(get_repo_directory)/api"; fi) \
  --plugin=protoc-gen-doc=$goenvbin/protoc-gen-doc --doc_out="$(get_repo_directory)/api/doc" --doc_opt=html,index.html \
  --proto_path "$(get_repo_directory)/api" "$(get_repo_directory)/api/"*.proto

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
