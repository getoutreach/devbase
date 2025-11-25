#!/usr/bin/env bash
# Linters for protobuf

# Why: Used by the script that calls us
# shellcheck disable=SC2034
extensions=(proto)

# Runs buf [...] on all versioned .proto files.
run_buf() {
  local buf_args cfg_path mise_bin possible_cfg_dirs repo_dir
  mise_bin="$(find_mise)"
  repo_dir="$(get_repo_directory)"
  possible_cfg_dirs=("$repo_dir"/api "$repo_dir")
  for pdir in "${possible_cfg_dirs[@]}"; do
    local pcfg="$pdir"/buf.yaml
    if [[ -f $pcfg ]]; then
      cfg_path="$pcfg"
      break
    fi
  done
  buf_args=()
  if [[ -n $cfg_path ]]; then
    buf_args+=(--config "$cfg_path")
  fi
  find_files_with_extensions "${extensions[@]}" | xargs printf -- '--path %s\n' | xargs -n40 "$mise_bin" exec buf@"$(get_tool_version buf)" -- buf "$@" "${buf_args[@]}"
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
