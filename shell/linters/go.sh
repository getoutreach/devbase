#!/usr/bin/env bash
# Linters for Golang
GOBIN="$DIR/../gobin.sh"

# Why: Used by the script that calls us
# shellcheck disable=SC2034
extensions=(go)

lintroller() {
  # The sed is used to strip the pwd from lintroller output,
  # which is currently prefixed with it.
  GOFLAGS=-tags=or_e2e,or_test "$GOBIN" \
    "github.com/getoutreach/lintroller/cmd/lintroller@v$(get_application_version "lintroller")" \
    -config scripts/golangci.yml ./... 2>&1 | sed "s#^$(pwd)/##"
}

linter() {
  run_linter "golangci-lint" \
    "$LINTER" --build-tags "or_e2e,or_test" --timeout 10m run ./...

  run_linter "lintroller" lintroller
}
