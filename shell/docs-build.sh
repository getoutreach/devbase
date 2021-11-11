#!/usr/bin/env bash
# Generate HTML documentation for Protobuf definitions and the Node.js gRPC client.

set -e

SCRIPTS_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"

# shellcheck source=./lib/bootstrap.sh
source "$SCRIPTS_DIR/lib/bootstrap.sh"
# shellcheck source=./lib/logging.sh
source "$SCRIPTS_DIR/lib/logging.sh"

ROOT_DIR="$(get_repo_directory)"
API_DIR="$ROOT_DIR/api"
NODEJS_CLIENT_DIR="$API_DIR/clients/node"
DOCS_DIR="$ROOT_DIR/apidocs"
PROTO_DOCS_DIR="$DOCS_DIR/proto"

info "Generating Documentation"

info_sub "Protobuf"

mkdir -p "$PROTO_DOCS_DIR"

if [[ -n $CI ]]; then
  pushd "$API_DIR" >/dev/null || fatal "Could not change directory to api"
  protoc --doc_out="$PROTO_DOCS_DIR" --doc_opt=html,index.html ./*.proto
  popd >/dev/null || fatal "Could not pop directory out of api"
else
  "$SCRIPTS_DIR/protoc.sh"
fi

if [[ -f "$NODEJS_CLIENT_DIR/package.json" ]]; then
  info_sub "TypeScript"

  TYPEDOC_ARGS=()
  if [[ -n $GIT_REVISION ]]; then
    TYPEDOC_ARGS+=("--gitRevision" "$GIT_REVISION")
  fi

  pushd "$NODEJS_CLIENT_DIR" >/dev/null || fatal "Could not change directory to Node.js client"
  yarn typedoc --out "$DOCS_DIR"/typescript "${TYPEDOC_ARGS[@]}" src/index.ts
  popd >/dev/null || fatal "Could not change directory out of Node.js client"
fi

# Allow files starting with an underscore to be viewed in GitHub Pages
touch "$DOCS_DIR/.nojekyll"
