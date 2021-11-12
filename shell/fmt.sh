#!/usr/bin/env bash
# Run various formatters for our source code
set -e

SCRIPTS_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"

# shellcheck source=./lib/bootstrap.sh
source "$SCRIPTS_DIR/lib/bootstrap.sh"

# Tools
JSONNETFMT=$("$SCRIPTS_DIR/gobin.sh" -p github.com/google/go-jsonnet/cmd/jsonnetfmt@v"$(get_application_version "jsonnetfmt")")
GOIMPORTS=$("$SCRIPTS_DIR/gobin.sh" -p golang.org/x/tools/cmd/goimports@v"$(get_application_version "goimports")")
SHELLFMTPATH="$DIR/shfmt.sh"
GOFMT="${GOFMT:-gofmt}"

# shellcheck source=./lib/logging.sh
source "$SCRIPTS_DIR/lib/logging.sh"

info "Running Formatters"

info_sub "goimports"
git ls-files "*.go" | xargs -n40 "$GOIMPORTS" -w

info_sub "gofmt"
git ls-files "*.go" | xargs -n40 gofmt -w -s

info_sub "go mod tidy"
go mod tidy

info_sub "jsonnetfmt"
git ls-files '*.(jsonnet|libsonnet)' | xargs -n40 "$JSONNETFMT"

info_sub "clang-format"
git ls-files "*.proto" | xargs -n40 "$SCRIPTS_DIR/clang-format.sh" -style=file -i

info_sub "shfmt"
git ls-files '*.sh' | xargs -n40 "$SHELLFMTPATH" -s -d

info_sub "prettier (yaml/json)"
# Only install node_modules the first time. It's up to the user
# to run it again if needed.
if [[ ! -d "node_modules" ]]; then
  rm -rf "node_modules"
  yarn
fi
yarn prettier --write "**/*.{yaml,yml,json}"

if has_feature "grpc"; then
  if has_grpc_client "node"; then
    nodeSourceDir="$(pwd)/api/clients/node"

    pushd "$nodeSourceDir" >/dev/null 2>&1 || exit 1
    # Only install node_modules the first time. It's up to the user
    # to run it again if needed.
    if [[ ! -d "node_modules" ]]; then
      rm -rf "node_modules"
      yarn
    fi

    info_sub "eslint (node)"
    yarn lint-fix

    info_sub "prettier (node)"
    yarn pretty-fix # When files are modified this returns 1.
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
