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

linter() {
  run_linter "go mod tidy" go_mod_tidy
  run_linter "golangci-lint" \
    "$DIR/golangci-lint.sh" --build-tags "or_e2e,or_test" --timeout 10m run ./...
  run_linter "lintroller" lintroller
}
