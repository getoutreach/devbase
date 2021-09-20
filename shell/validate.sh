#!/usr/bin/env bash
set -e -o pipefail

# The linter is flaky in some environments so we allow it to be overridden.
# Also, if your editor already supports linting, you can make your tests run
# faster at little cost with:
# `LINTER=/bin/true make test``
DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
LINTER="${LINTER:-"$DIR/golangci-lint.sh"}"
SHELLFMTPATH="$DIR/shfmt.sh"
SHELLCHECKPATH="$DIR/shellcheck.sh"
GOBIN="$DIR/gobin.sh"

# shellcheck source=./lib/logging.sh
source "$DIR/lib/logging.sh"
# shellcheck source=./lib/runtimes.sh
source "$DIR/lib/runtimes.sh"
# shellcheck source=./lib/bootstrap.sh
source "$DIR/lib/bootstrap.sh"

info "Running linters"

# Run shellcheck on shell-scripts, only if installed.
info_sub "shellcheck"
# Make sure to ignore the monitoring/.terraform directory
# shellcheck disable=SC2038
if ! git ls-files '*.sh' | xargs -n40 "$SHELLCHECKPATH" -x -P SCRIPTDIR; then
  error "shellcheck failed on some files. Run 'make fmt' to fix."
  exit 1
fi

info_sub "shfmt"
if ! git ls-files '*.sh' | xargs -n40 "$SHELLFMTPATH" -s -d; then
  error "shfmt failed on some files. Run 'make fmt' to fix."
  exit 1
fi

# Validators to run when not using a library
if ! has_feature "library"; then
  info_sub "terraform"
  for tfdir in deployments monitoring; do
    if ! "$DIR"/terraform.sh fmt -diff -check "$tfdir"; then
      error "terraform fmt $tfdir failed on some files. Run 'make fmt' to fix."
      exit 1
    fi
  done
fi

info_sub "clang-format"
if ! git ls-files '*.proto' | xargs -n40 "$DIR/clang-format-validate.sh"; then
  error "clang-format failed on some files. Run 'make fmt' to fix."
  exit 1
fi

info_sub "golangci-lint"
"$LINTER" --build-tags "$TEST_TAGS" --timeout 10m run ./...

if [[ "$OSS" == "false" ]]; then
  info_sub "lintroller"
  # The sed is used to strip the pwd from lintroller output, which is currently prefixed with it.
  "$GOBIN" "github.com/getoutreach/lintroller/cmd/lintroller@v$(get_application_version "lintroller")" \
    -config scripts/golangci.yml ./... 2>&1 | sed "s#^$(pwd)/##"
fi

# GRPC client validation
if has_feature "grpc"; then
  if has_grpc_client "node"; then
    CLIENTS_DIR="$(get_repo_directory)/api/clients"

    nodeSourceDir="$CLIENTS_DIR/node"

    run_node_command "$nodeSourceDir" yarn install --frozen-lockfile

    info_sub "prettier (node)"
    run_node_command "$nodeSourceDir" yarn pretty

    info_sub "eslint (node)"
    run_node_command "$nodeSourceDir" yarn lint
  fi
fi
