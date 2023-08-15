#!/usr/bin/env bash
# Linters for Bash
SHELLFMTPATH="$DIR/shfmt.sh"
SHELLCHECKPATH="$DIR/shellcheck.sh"

# Why: Used by the script that calls us
# shellcheck disable=SC2034
extensions=(sh bash bats)

shellcheck_linter() {
  find_files_with_extensions "${extensions[@]}" | xargs -n40 "$SHELLCHECKPATH" -x -P SCRIPTDIR
}

shellfmt_linter() {
  find_files_with_extensions "${extensions[@]}" | xargs -n40 "$SHELLFMTPATH" -s -d
}

shellfmt_formatter() {
  find_files_with_extensions "${extensions[@]}" | xargs -n40 "$SHELLFMTPATH" -w -l
}

linter() {
  run_command "shellcheck" shellcheck_linter || return 1
  run_command "shellfmt" shellfmt_linter || return 1
}

formatter() {
  run_command "shellfmt" shellfmt_formatter
}
