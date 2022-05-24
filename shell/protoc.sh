#!/usr/bin/env bash

SCRIPTS_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
GOBIN="$SCRIPTS_DIR/gobin.sh"

# shellcheck source=./lib/logging.sh
source "$SCRIPTS_DIR/lib/logging.sh"

# shellcheck source=./lib/bootstrap.sh
source "$SCRIPTS_DIR/lib/bootstrap.sh"

PROTO_DOCS_DIR="$(get_repo_directory)/apidocs/proto"

PROTOC_IMPORTS_BASE_PATH="$HOME/.outreach/.protoc-imports"
mkdir -p "$PROTOC_IMPORTS_BASE_PATH"

info "Ensuring protoc imports are avaliable locally"

imports=$(get_list "go-protoc-imports")

# Add default imports so they can get dealt with in the following loop just like custom imports.
if [[ has_feature "validation" ]]; then
  imports="${imports}{\"module\":\"github.com/envoyproxy/protoc-gen-validate@v0.6.7\",\"path\":\"\"}$'\n'"
fi

protoc_imports=()
for import in $(get_list "go-protoc-imports"); do
  module_version_str=$(jq -r .module <<<"$import")
  info_sub "$module_version_str"

  module_version_arr=(${module_version_str//@/ })
  module=${module_version_arr[0]}
  version="latest"

  if [[ ${#module_version_arr[@]} -eq 2 ]]; then
    version=${module_version_arr[1]}
  fi

  if [[ $version == "latest" ]]; then
    version="$(gh release -R $module list -L 1 | awk '{print $1}')"
  fi

  import_path="$PROTOC_IMPORTS_BASE_PATH/$module/$version"
  path=$(jq -r .path <<<"$import")
  path=${path#"/"} # Trim / prefix if it exists.

  # Add import to list
  protoc_imports+=("$import_path/$path")
  
  # Check to see if we already have this version locally and skip cloning
  # if we already do.
  if [[ -d "$import_path" ]]; then
    continue
  fi

  # Clone import into the import path since we don't have it already
  mkdir -p "$import_path"
  git clone --depth 1 --branch "$version" "$module" "$import_path"
done

info "Ensuring protoc plugins are installed"

info_sub "protoc-gen-validate"
protoc_gen_validate=$($GOBIN -p github.com/envoyproxy/protoc-gen-validate@v0.6.7)

info_sub "protoc-gen-go"
protoc_gen_go=$($GOBIN -p google.golang.org/protobuf/cmd/protoc-gen-go@v1.28)

info_sub "protoc-gen-go-grpc"
protoc_gen_go_grpc=$($GOBIN -p google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.2)

info_sub "protoc-gen-doc"
protoc_gen_doc=$($GOBIN -p github.com/pseudomuto/protoc-gen-doc/cmd/protoc-gen-doc@v1.5.1)

info "Generating GRPC Clients"

# Create the language specific clients
info_sub "go"
exec protoc \
  --plugin=protoc-gen-go=$protoc_gen_go --plugin=protoc-gen-go-grpc=$protoc_gen_go_grpc \
  --go_out=. --go_opt=paths=source_relative \
  --go-grpc_out=. --go-grpc_opt=paths=source_relative \
  $(for i in "${protoc_imports[@]}"; do echo "--proto_path=$i"; done) \
  $(if has_feature "validation"; then echo "--plugin=protoc-gen-validate=$protoc_gen_validate --validate_out=lang=go:$(get_repo_directory)/api"; fi) \
  --plugin=protoc-gen-doc=$protoc_gen_doc --doc_out="$(get_repo_directory)/api/doc" --doc_opt=html,index.html \
  --proto_path "$(get_repo_directory)/api" "$(get_repo_directory)/api/"*.proto

mkdir -p "$PROTO_DOCS_DIR"
mv "$(get_repo_directory)"/api/doc/index.html "$PROTO_DOCS_DIR"

# Legacy below this, need to deal with.
IMAGE="gcr.io/outreach-docker/protoc:1.37_2"

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
