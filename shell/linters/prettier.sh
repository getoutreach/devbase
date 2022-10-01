#!/usr/bin/env bash
# Generic linter/formatter for prettier

# Note: This is called from the perspective of shell/
# shellcheck source=../languages/nodejs.sh
source "$DIR/languages/nodejs.sh"

# Why: Used by the script that calls us
# shellcheck disable=SC2034
extensions=(yaml yml json md)

prettier_linter() {
  yarn_install_if_needed >/dev/null
  git ls-files '*.yaml' '*.yml' '*.json' '*.md' | xargs -n40 "node_modules/.bin/prettier" -l --loglevel warn
}

prettier_formatter() {
  yarn_install_if_needed >/dev/null
  git ls-files '*.yaml' '*.yml' '*.json' '*.md' | xargs -n40 "node_modules/.bin/prettier" --write --loglevel warn
}

linter() {
  run_command "prettier" prettier_linter
}

formatter() {
  run_command "prettier" prettier_formatter
}
