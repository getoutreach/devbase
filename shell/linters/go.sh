#!/usr/bin/env bash
# Linters for Golang

# Why: Used by the script that calls us
# shellcheck disable=SC2034
extensions=("go")

lintroller() {
  "$DIR/lintroller.sh" ./...
}

go_mod_tidy() {
  go mod tidy

  # We only ever error on this in CI, since it's updated when we run the above...
  # Eventually we can do `go mod tidy -check` or something else:
  # https://github.com/golang/go/issues/27005
  #
  # Skip when go.sum doesn't exist, because this causes errors. This can
  # happen when go.mod has no dependencies
  if [[ -n $CI ]] && [[ -e "go.sum" ]]; then
    git diff --exit-code go.{mod,sum} || fatal "go.{mod,sum} are out of date, please run 'go mod tidy' and commit the result"
  fi
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
  run_command "go mod tidy" go_mod_tidy || return 1
  run_command "golangci-lint" \
    "$DIR/golangci-lint.sh" --build-tags "or_e2e,or_test" --timeout 10m run --out-format colored-line-number ./... || return 1
  run_command "lintroller" lintroller || return 1
}

formatter() {
  run_command "go mod tidy" go_mod_tidy || return 1
  run_command "goimports" goimports || return 1
  run_command "gofmt" gofmt || return 1
}
