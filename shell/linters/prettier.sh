#!/usr/bin/env bash
# Generic linter/formatter for prettier

# Note: This is called from the perspective of shell/
# shellcheck source=../languages/nodejs.sh
source "$DIR/languages/nodejs.sh"

# Why: Used by the script that calls us
# shellcheck disable=SC2034
extensions=(yaml yml json md ts)

PRETTIER="node_modules/.bin/prettier"

prettier_log_level_flag() {
  if [[ $("$PRETTIER" --version) =~ ^2 ]]; then
    echo "--loglevel"
  else
    echo "--log-level"
  fi
}

prettier_linter() {
  yarn_install_if_needed >/dev/null

  local log_level_flag
  log_level_flag="$(prettier_log_level_flag)"

  find_files_with_extensions "${extensions[@]}" | xargs -n40 "$PRETTIER" --list-different "$log_level_flag" log
  return $?
}

prettier_formatter() {
  yarn_install_if_needed >/dev/null

  local log_level_flag
  log_level_flag="$(prettier_log_level_flag)"

  find_files_with_extensions "${extensions[@]}" | xargs -n40 "$PRETTIER" --write "$log_level_flag" warn
}

linter() {
  run_command "prettier" prettier_linter
}

formatter() {
  run_command "prettier" prettier_formatter
}
