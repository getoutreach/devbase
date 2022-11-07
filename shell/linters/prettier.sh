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
  git ls-files '*.yaml' '*.yml' '*.json' '*.md' '*.ts' | xargs -n40 "node_modules/.bin/prettier" -l --loglevel log
  
  # Print out a friendlier error message, because prettier will just print a list of files
  code=$?
  if [[ $code == 1 ]]; then
	  # Clear lines above and below because magic terminal stuff going on
	  echo
	  echo "prettier_linter: the above files have not been formatted. Please run \`make fmt\` to fix this."
	  echo
  fi
  
  return $code
}

prettier_formatter() {
  yarn_install_if_needed >/dev/null
  git ls-files '*.yaml' '*.yml' '*.json' '*.md' '*.ts' | xargs -n40 "node_modules/.bin/prettier" --write --loglevel warn
}

linter() {
  run_command "prettier" prettier_linter
}

formatter() {
  run_command "prettier" prettier_formatter
}
