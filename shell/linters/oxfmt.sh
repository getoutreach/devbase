#!/usr/bin/env bash
# Generic linter/formatter for oxfmt

# Why: Used by the script that calls us
# shellcheck disable=SC2034
extensions=(yaml yml json md ts)

oxfmt_linter() {
  find_files_with_extensions "${extensions[@]}" | xargs -n40 "$DIR/oxfmt.sh" --check --list-different
}

oxfmt_formatter() {
  find_files_with_extensions "${extensions[@]}" | xargs -n40 "$DIR/oxfmt.sh" --write
}

linter() {
  run_command "oxfmt" oxfmt_linter
}

formatter() {
  run_command "oxfmt" oxfmt_formatter
}
