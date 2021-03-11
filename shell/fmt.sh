#!/usr/bin/env bash
# Run various formatters for our source code
set -e

SCRIPTS_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"

# Tools
JSONNETFMT=$("$SCRIPTS_DIR/gobin.sh" -p github.com/google/go-jsonnet/cmd/jsonnetfmt@v0.16.0)
GOIMPORTS=$("$SCRIPTS_DIR/gobin.sh" -p golang.org/x/tools/cmd/goimports@v0.1.0)
GOFMT="${GOFMT:-gofmt}"

# shellcheck source=./lib/runtimes.sh
source "$SCRIPTS_DIR/lib/runtimes.sh"
# shellcheck source=./lib/logging.sh
source "$SCRIPTS_DIR/lib/logging.sh"

info "Running Formatters"

info_sub "goimports"
find . -path ./vendor -prune -o -type f -name '*.go' \
  -exec "$GOIMPORTS" -w {} +

info_sub "gofmt"
find . -path ./vendor -prune -o -type f -name '*.go' \
  -exec gofmt -w -s {} +

info_sub "go mod tidy"
go mod tidy

info_sub "jsonnetfmt"
find . -name '*.jsonnet' -exec "$JSONNETFMT" -i {} +

info_sub "clang-format"
find . -path ./api/clients -prune -o -name '*.proto' -exec "$SCRIPTS_DIR/clang-format.sh" -style=file -i {} \;

info_sub "shfmt"
find . -path ./vendor -prune -o -name node_modules -type d \
  -prune -o -type f -name '*.sh' -exec "$SCRIPTS_DIR/shfmt.sh" -w -l {} +

info_sub "Prettier (yaml/json)"
run_node_command "$SCRIPTS_DIR/.." yarn
run_node_command "$SCRIPTS_DIR/.." yarn prettier --write "**/*.{yaml,yml,json}"
for tfdir in deployments monitoring; do
  "$SCRIPTS_DIR/terraform.sh" fmt "$SCRIPTS_DIR/../$tfdir"
done
