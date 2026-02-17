#!/usr/bin/env bash
# Linters+Formatters for TOML files

# Why: Used by the script that calls us
# shellcheck disable=SC2034
extensions=(toml)

mise_fmt() {
  # mise fmt does not take file arguments
  mise fmt --all
}

tombi_format() {
  find_files_with_extensions "${extensions[@]}" | TOMBI_NO_CACHE=true TOMBI_OFFLINE=true xargs -n40 "$DIR/tombi.sh" format --quiet
}

tombi_format_check() {
  find_files_with_extensions "${extensions[@]}" | TOMBI_NO_CACHE=true TOMBI_OFFLINE=true xargs -n40 "$DIR/tombi.sh" format --check --quiet
}

tombi_lint() {
  find_files_with_extensions "${extensions[@]}" | xargs -n40 "$DIR/tombi.sh" lint
}

linter() {
  run_command "tombi format (check)" tombi_format_check || return 1
}

formatter() {
  run_command "mise fmt" mise_fmt
  run_command "tombi format" tombi_format || return 1
}
