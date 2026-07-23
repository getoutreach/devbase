#!/usr/bin/env bash
# Linters for protobuf

# Why: Used by the script that calls us
# shellcheck disable=SC2034
extensions=(proto)

# Runs buf [...] on all versioned .proto files.
run_buf() {
  local mise_bin
  mise_bin="$(find_mise)"
  # buf only allows one path (file/folder) to be passed to it in the args.
  # However, you can get around this by passing multiple `--path <path/to.proto>`
  # flags, which requires an extra `xargs printf` to generate.
  find_files_with_extensions "${extensions[@]}" | xargs printf -- '--path %s\n' |
    xargs -n40 "$mise_bin" exec buf@"$(devbase_tool_version_from_mise buf)" -- buf "$@"
}

buf_linter() {
  run_buf format --exit-code --diff
}

buf_formatter() {
  run_buf format --write
}

buf_lint_linter() {
  run_buf lint
}

linter() {
  run_command "buf format" buf_linter
  run_command "buf lint" buf_lint_linter
}

formatter() {
  run_command "buf" buf_formatter
}
