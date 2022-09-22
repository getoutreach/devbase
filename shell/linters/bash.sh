#!/usr/bin/env bash
# Linters for Bash
SHELLFMTPATH="$DIR/shfmt.sh"
SHELLCHECKPATH="$DIR/shellcheck.sh"

# Why: Used by the script that calls us
# shellcheck disable=SC2034
extensions=(sh bash)

shellcheck_linter() {
  git ls-files '*.sh' | xargs -n40 "$SHELLCHECKPATH" -x -P SCRIPTDIR
}

shellfmt_linter() {
  git ls-files '*.sh' | xargs -n40 "$SHELLFMTPATH" -s -d
}

shellfmt_formatter() {
  git ls-files '*.sh' | xargs -n40 "$SHELLFMTPATH" -w -l
}

linter() {
  run_command "shellcheck" shellcheck_linter
  run_command "shellfmt" shellfmt_linter
}

formatter() {
  run_command "shellfmt" shellfmt_formatter
}
