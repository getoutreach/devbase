#!/usr/bin/env bash
# Linters+Formatters for jsonnet

# Why: Used by the script that calls us
# shellcheck disable=SC2034
extensions=(jsonnet libsonnet)

jsonnetfmt() {
  find_files_with_extensions "${extensions[@]}" |
    xargs_mise_exec 40 go-jsonnet@v"$(get_tool_version "jsonnetfmt")" \
      jsonnetfmt -i
}

linter() {
  true # No linters yet
}

formatter() {
  run_command "jsonnetfmt" jsonnetfmt
}
