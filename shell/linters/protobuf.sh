#!/usr/bin/env bash
# Linters for protobuf

# Why: Used by the script that calls us
# shellcheck disable=SC2034
extensions=(proto)

# Runs buf [...] on all versioned .proto files.
run_buf() {
  local mise_bin
  mise_bin="$(find_mise)"
  find_files_with_extensions "${extensions[@]}" | xargs -n1 "$mise_bin" exec buf@"$(get_tool_version buf)" -- buf "$@"
}

buf_linter() {
  run_buf format --exit-code --diff
}

buf_formatter() {
  run_buf format --write
}

linter() {
  run_command "buf" buf_linter
}

formatter() {
  run_command "buf" buf_formatter
}
