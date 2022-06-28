#!/usr/bin/env bash
# Linters for Bash
SHELLFMTPATH="$DIR/shfmt.sh"
SHELLCHECKPATH="$DIR/shellcheck.sh"

# Why: Used by the script that calls us
# shellcheck disable=SC2034
extensions=(sh bash)

shellcheck() {
  if ! git ls-files '*.sh' | xargs -n40 "$SHELLCHECKPATH" -x -P SCRIPTDIR; then
    error "shellcheck failed on some files. Run 'make fmt' to fix."
    exit 1
  fi
}

shellfmt() {
  if ! git ls-files '*.sh' | xargs -n40 "$SHELLFMTPATH" -s -d; then
    error "shfmt failed on some files. Run 'make fmt' to fix."
    exit 1
  fi
}

linter() {
  run_linter "shellcheck" shellcheck
  run_linter "shellfmt" shellfmt
}
