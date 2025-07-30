#!/usr/bin/env bash
# Generic linter/formatter for prettier

# Note: This is called from the perspective of shell/
# shellcheck source=../languages/nodejs.sh
source "$DIR/languages/nodejs.sh"

# shellcheck source=../lib/mise.sh
source "$DIR/lib/mise.sh"

# Why: Used by the script that calls us
# shellcheck disable=SC2034
extensions=(yaml yml json md ts)

find_and_install_prettier_if_needed() {
  PRETTIER="node_modules/.bin/prettier"
  if [[ ! -f $PRETTIER && (! -f package.json || "$(gojq --raw-output .devDependencies.prettier package.json)" == "null") ]]; then
    mise_install_if_needed npm:prettier
    # Try to find prettier installed via mise
    PRETTIER="$(mise which prettier)"
    if [[ -z $PRETTIER ]]; then
      fatal "prettier not found in repo, make sure 'npm:prettier' is defined in 'mise.toml' and you have run 'mise install'"
    fi
  fi

  if [[ $PRETTIER =~ ^node_modules/ ]]; then
    yarn_install_if_needed >/dev/null
  fi
}

prettier_log_level_flag() {
  if [[ $("$PRETTIER" --version) =~ ^2 ]]; then
    echo "--loglevel"
  else
    echo "--log-level"
  fi
}

prettier_linter() {
  find_and_install_prettier_if_needed

  local log_level_flag
  log_level_flag="$(prettier_log_level_flag)"

  find_files_with_extensions "${extensions[@]}" | xargs -n40 "$PRETTIER" --check "$log_level_flag" log
  return $?
}

prettier_formatter() {
  find_and_install_prettier_if_needed

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
