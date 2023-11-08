#!/usr/bin/env bash
# Generic linter/formatter for prettier

# Note: This is called from the perspective of shell/
# shellcheck source=../languages/nodejs.sh
source "$DIR/languages/nodejs.sh"

# Why: Used by the script that calls us
# shellcheck disable=SC2034
extensions=(yaml yml json md ts)

prettier_linter() {
  yarn_install_if_needed >/dev/null
  find_files_with_extensions "${extensions[@]}" | xargs -n40 "node_modules/.bin/prettier" -l --log-level log
  return $?
}

prettier_formatter() {
  yarn_install_if_needed >/dev/null
  find_files_with_extensions "${extensions[@]}" | xargs -n40 "node_modules/.bin/prettier" --write --log-level warn
}

linter() {
  run_command "prettier" prettier_linter
}

formatter() {
  run_command "prettier" prettier_formatter
}
