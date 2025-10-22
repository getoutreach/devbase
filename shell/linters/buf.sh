#!/usr/bin/env bash
# Linters for protobuf

# Why: Used by the script that calls us
# shellcheck disable=SC2034
extensions=(proto)

buf_linter() {
  # Why: We're OK with this.
  # shellcheck disable=SC2155
  local PROTOFMT=$("$DIR/gobin.sh" -p github.com/bufbuild/buf/cmd/buf@v"$(get_tool_version "buf")")
  find_files_with_extensions "${extensions[@]}" | xargs -n1 "$PROTOFMT" lint
}

buf_formatter() {
  # Why: We're OK with this.
  # shellcheck disable=SC2155
  local PROTOFMT=$("$DIR/gobin.sh" -p github.com/bufbuild/buf/cmd/buf@v"$(get_tool_version "buf")")
  find_files_with_extensions "${extensions[@]}" | xargs -n1 "$PROTOFMT" lint
}

linter() {
  run_command "buf lint" buf_linter
}

formatter() {
  run_command "buf lint" buf_formatter
}
