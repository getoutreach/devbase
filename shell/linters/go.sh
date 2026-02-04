#!/usr/bin/env bash
# Linters for Golang

# Why: Used by the script that calls us
# shellcheck disable=SC2034
extensions=("go")

lintroller() {
  "$DIR/lintroller.sh" ./...
}

goimports() {
  find_files_with_extensions "${extensions[@]}" | xargs -n40 "$DIR/goimports.sh" -w
}

gofmt() {
  find_files_with_extensions "${extensions[@]}" | xargs -n40 "$(command -v gofmt)" -s -w
}

gofumpt() {
  # gofumpt has flags compatible with gofmt, however:
  # -s is deprecated as it is always enabled
  find_files_with_extensions "${extensions[@]}" | GITHUB_TOKEN="$(github_token)" xargs -n40 "$DIR/gofumpt.sh" -w
}

linter() {
  run_command "go mod tidy" go mod tidy -diff || return 1
  # gofmt/goimports/gofumpt checking is done by golangci-lint
  run_command "golangci-lint" \
    "$DIR/golangci-lint.sh" --build-tags "or_e2e,or_test" --timeout 10m run ./... || return 1
  run_command "lintroller" lintroller || return 1
}

formatter() {
  local goFormatter
  goFormatter="$(stencil_arg go.formatter)"
  if [[ -f "$(get_repo_directory)/go.work" ]]; then
    run_command "go work use" go work use || return 1
  fi
  run_command "go mod tidy" go mod tidy || return 1
  if [[ -z $goFormatter || $goFormatter == "null" || $goFormatter == "gofmt" ]]; then
    run_command goimports goimports || return 1
    run_command gofmt gofmt || return 1
  elif [[ $goFormatter == gofumpt ]]; then
    run_command gofumpt gofumpt || return 1
  else
    fatal "Unknown Go formatter: $goFormatter"
  fi
}
