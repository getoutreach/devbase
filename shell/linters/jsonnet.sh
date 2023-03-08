#!/usr/bin/env bash
# Linters+Formatters for jsonnet

# Why: Used by the script that calls us
# shellcheck disable=SC2034
extensions=(jsonnet libsonnet)

jsonnetfmt() {
  # Why: We're OK with this.
  # shellcheck disable=SC2155
  local JSONNETFMT=$("$DIR/gobin.sh" -p github.com/google/go-jsonnet/cmd/jsonnetfmt@v"$(get_application_version "jsonnetfmt")")
  find_files_with_extensions "${extensions[@]}" | xargs -n40 "$JSONNETFMT" -i
}

linter() {
  true # No linters yet
}

formatter() {
  run_command "jsonnetfmt" jsonnetfmt
}
