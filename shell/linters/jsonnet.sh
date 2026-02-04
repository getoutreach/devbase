#!/usr/bin/env bash
# Linters+Formatters for jsonnet

# Why: Used by the script that calls us
# shellcheck disable=SC2034
extensions=(jsonnet libsonnet)

jsonnetfmt() {
  find_files_with_extensions "${extensions[@]}" |
    xargs_mise_exec_tool_with_bin 40 go-jsonnet jsonnetfmt -i
}

linter() {
  true # No linters yet
}

formatter() {
  run_command "jsonnetfmt" jsonnetfmt
}
