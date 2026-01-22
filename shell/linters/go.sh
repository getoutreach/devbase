#!/usr/bin/env bash
# Linters for Golang

# Why: Used by the script that calls us
# shellcheck disable=SC2034
extensions=("go")

lintroller() {
  "$DIR/lintroller.sh" ./...
}

goimports() {
  # Why: We're OK with this.
  # shellcheck disable=SC2155
  local GOIMPORTS=$("$DIR/gobin.sh" -p golang.org/x/tools/cmd/goimports@v"$(get_application_version "goimports")")
  find_files_with_extensions "${extensions[@]}" | xargs -n40 "$GOIMPORTS" -w
}

gofmt() {
  find_files_with_extensions "${extensions[@]}" | xargs -n40 "$(command -v gofmt)" -s -w
}

linter() {
  run_command "go mod tidy" go mod tidy -diff || return 1
  run_command "golangci-lint" \
    "$DIR/golangci-lint.sh" --build-tags "or_e2e,or_test" --timeout 10m run ./... || return 1
  run_command "lintroller" lintroller || return 1
}

formatter() {
  if [[ -f "$(get_repo_directory)/go.work" ]]; then
    run_command "go work use" go work use || return 1
  fi
  run_command "go mod tidy" go mod tidy || return 1
  run_command "goimports" goimports || return 1
  run_command "gofmt" gofmt || return 1
}
