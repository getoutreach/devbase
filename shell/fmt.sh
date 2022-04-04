#!/usr/bin/env bash
# Run various formatters for our source code
set -e

SCRIPTS_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"

# shellcheck source=./lib/bootstrap.sh
source "$SCRIPTS_DIR/lib/bootstrap.sh"
# shellcheck source=./languages/nodejs.sh
source "$SCRIPTS_DIR/languages/nodejs.sh"
# shellcheck source=./lib/logging.sh
source "$SCRIPTS_DIR/lib/logging.sh"

# Tools
JSONNETFMT=$("$SCRIPTS_DIR/gobin.sh" -p github.com/google/go-jsonnet/cmd/jsonnetfmt@v"$(get_application_version "jsonnetfmt")")
GOIMPORTS=$("$SCRIPTS_DIR/gobin.sh" -p golang.org/x/tools/cmd/goimports@v"$(get_application_version "goimports")")
SHELLFMTPATH="$SCRIPTS_DIR/shfmt.sh"
GOFMT="${GOFMT:-gofmt}"

info "Running Formatters"

info_sub "goimports"
find . -path ./vendor -prune -o -path .snapshots -prune -o -type f -name '*.go' \
  -exec "$GOIMPORTS" -w {} +

info_sub "gofmt"
find . -path ./vendor -prune -o -type f -name '*.go' \
  -exec gofmt -w -s {} +

info_sub "go mod tidy"
go mod tidy

info_sub "jsonnetfmt"
find . -name '*.jsonnet' -exec "$JSONNETFMT" -i {} +

info_sub "clang-format"
find . -path "$(get_repo_directory)/api/clients" -prune -o -name '*.proto' -exec "$SCRIPTS_DIR/clang-format.sh" -style=file -i {} \;

info_sub "shfmt"
find . -path ./vendor -prune -o -path ./.bootstrap -prune -o -name node_modules -type d \
  -prune -o -type f -name '*.sh' -exec "$SHELLFMTPATH" -w -l {} +

info_sub "prettier (yaml/json/md)"
yarn_install_if_needed
yarn prettier --write "**/*.{yaml,yml,json,md}" >/dev/null

if has_feature "grpc"; then
  if has_grpc_client "node"; then
    nodeSourceDir="$(pwd)/api/clients/node"
    pushd "$nodeSourceDir" >/dev/null 2>&1 || exit 1
    yarn_install_if_needed

    info_sub "eslint (node)"
    yarn lint-fix >/dev/null

    info_sub "prettier (node)"
    yarn pretty-fix >/dev/null # When files are modified this returns 1.
    popd >/dev/null 2>&1
  fi
fi

if ! has_feature "library"; then
  if [[ -e deployments ]] && [[ -e monitoring ]]; then
    for tfdir in deployments monitoring; do
      "$SCRIPTS_DIR/terraform.sh" fmt "$tfdir"
    done
  fi
fi
