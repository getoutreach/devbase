#!/usr/bin/env bash
# Linters for protobuf

# Why: Used by the script that calls us
# shellcheck disable=SC2034
extensions=(proto)

load_tool() {
  "$DIR/gobin.sh" -p github.com/bufbuild/buf/cmd/buf@v"$(get_tool_version "buf")"
}

buf_linter() {
  # Why: We're OK with this.
  # shellcheck disable=SC2155
  local PROTOFMT=$(load_tool)
  find_files_with_extensions "${extensions[@]}" | xargs -n1 "$PROTOFMT" format --exit-code --diff
}

buf_formatter() {
  # Why: We're OK with this.
  # shellcheck disable=SC2155
  local PROTOFMT=$(load_tool)
  find_files_with_extensions "${extensions[@]}" | xargs -n1 "$PROTOFMT" format -w
}

buf_lint_linter() {
  # Why: We're OK with this.
  # shellcheck disable=SC2155
  local PROTOFMT=$(load_tool)
  find_files_with_extensions "${extensions[@]}" | xargs -n1 "$PROTOFMT" lint --path
}

linter() {
  run_command "buf" buf_linter
  run_command "buf" buf_lint_linter
}

formatter() {
  run_command "buf" buf_formatter
}
