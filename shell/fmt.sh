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
JSONNETFMT=$("$SCRIPTS_DIR/gobin.sh" -p github.com/google/go-jsonnet/cmd/jsonnetfmt@"$(get_application_version "jsonnetfmt")")
GOIMPORTS=$("$SCRIPTS_DIR/gobin.sh" -p golang.org/x/tools/cmd/goimports@v"$(get_application_version "goimports")")
SHELLFMTPATH="$SCRIPTS_DIR/shfmt.sh"
GOFMT="${GOFMT:-gofmt}"
PROTOFMT=$("$SCRIPTS_DIR/gobin.sh" -p github.com/bufbuild/buf/cmd/buf@v"$(get_application_version "buf")")

info "Running Formatters"

# for_all_files runs the provided command against all files that
# match the provided glob. This is powered by find, and thus the glob
# must match whatever `-name` supports matching against. Directories
# provided to `skip_directories` are automatically skipped.
for_all_files() {
  local skip_directories=(
    # Snapshot testing for templates
    '*.snapshots*'

    # Go vendoring, unsupported by attempt to skip it anyways.
    # Also used by ruby.
    "./vendor"

    # Skip gRPC clients
    "./api/clients"

    # Skip devbase, when it's embedded
    "./.bootstrap"

    # Skip node modules
    "*node_modules*"

    # Skip git internals
    "./.git"
  )
  local glob="$1"
  shift
  local command=("$@")

  # create arguments for each directory we're skipping
  local find_args=()
  for dir in "${skip_directories[@]}"; do
    find_args+=(-path "$dir" -prune -o)
  done

  # only include files, exec the command
  find_args+=(-type f -name "$glob" -exec "${command[@]}" {} +)

  find . "${find_args[@]}"
}

info_sub "goimports (.go)"
for_all_files '*.go' "$GOIMPORTS" -w

info_sub "gofmt (.go)"
for_all_files '*.go' gofmt -w -s

info_sub "go mod tidy"
go mod tidy

info_sub "jsonnetfmt (.jsonnet/.libsonnet)"
for ext in "jsonnet" "libsonnet"; do
  for_all_files '*.'${ext} "$JSONNETFMT" -i
done

info_sub "buf (.proto)"
"$PROTOFMT" format -w

info_sub "shfmt (.sh)"
for_all_files '*.sh' "$SHELLFMTPATH" -w -l

info_sub "prettier (.yaml/.yml/.json/.md)"
yarn_install_if_needed
for ext in "yaml" "yml" "json" "md"; do
  for_all_files '*.'${ext} "node_modules/.bin/prettier" --write --loglevel warn
done

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
    info_sub "terraform fmt (.tf/.tfvars)"
    for tfdir in deployments monitoring; do
      terraform fmt "$tfdir"
    done
  fi
fi
