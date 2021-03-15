#!/usr/bin/env bash
set -e

# The linter is flaky in some environments so we allow it to be overridden.
# Also, if your editor already supports linting, you can make your tests run
# faster at little cost with:
# `LINTER=/bin/true make test``
DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
LINTER="${LINTER:-"$DIR/golangci-lint.sh"}"
SHELLFMTPATH="$DIR/shfmt.sh"
SHELLCHECKPATH="$DIR/shellcheck.sh"

# shellcheck source=./lib/logging.sh
source "$DIR/lib/logging.sh"
# shellcheck source=./lib/runtimes.sh
source "$DIR/lib/runtimes.sh"
# shellcheck source=./lib/bootstrap.sh
source "$DIR/lib/bootstrap.sh"

# Run shellcheck on shell-scripts, only if installed.
info "Running shellcheck"
# Make sure to ignore the monitoring/.terraform directory
# shellcheck disable=SC2038
if ! git ls-files '*.sh' | xargs -n40 "${SHELLCHECKPATH}" -x -P SCRIPTDIR; then
  error "shellcheck failed on some files. Run 'make fmt' to fix."
  exit 1
fi

info "Running shfmt"
if ! git ls-files '*.sh' | xargs -n40 "$SHELLFMTPATH" -s -d; then
  error "shfmt failed on some files. Run 'make fmt' to fix."
  exit 1
fi

# Validators to run when not using a library
if [[ "$(yq -r .library <"$(get_service_yaml)")" != "true" ]]; then
  info "Running terraform fmt ($(get_application_version "terraform"))"
  for tfdir in deployments monitoring; do
    if ! "$DIR"/terraform.sh fmt -diff -check "$tfdir"; then
      error "terraform fmt $tfdir failed on some files. Run 'make fmt' to fix."
      exit 1
    fi
  done
fi

info "Running clang-format"
if ! find . -path ./api/clients -prune -o -name '*.proto' -exec "$DIR"/clang-format-validate.sh {} +; then
  error "clang-format failed on some files. Run 'make fmt' to fix."
  exit 1
fi

info "Running Go linter"
"$LINTER" --build-tags "$TEST_TAGS" --timeout 10m run ./...
CLIENTS_DIR="$DIR/../api/clients"

# GRPC client validation
if [[ "$(yq -r .grpc <"$(get_service_yaml)")" == "true" ]]; then
  CLIENTS_DIR="$DIR/../../api/clients"
  if has_grpc_client "node"; then
    nodeSourceDir="$CLIENTS_DIR/node"

    run_node_command "$nodeSourceDir" yarn install --frozen-lockfile

    info "Running Prettier (Node.js)"
    run_node_command "$nodeSourceDir" yarn pretty

    info "Running ESLint (Node.js)"
    run_node_command "$nodeSourceDir" yarn lint
  fi
fi
