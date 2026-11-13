#!/usr/bin/env bash
# Linter for the CODEOWNERS file. It validates the owners on GitHub only
# when CODEOWNERS changed since the merge base with the default branch.
# See docs/codeowners.md.

# shellcheck source=../lib/devbase.sh
source "$DIR/lib/devbase.sh"

# CODEOWNERS has no extension. An empty list matches every file, so
# this linter always runs, and linter() skips a repository without a
# CODEOWNERS file.
# Why: Used by the script that calls us
# shellcheck disable=SC2034
extensions=()

# codeowners_file echoes the path of the first CODEOWNERS file in a
# location that GitHub reads, or fails when there is none.
codeowners_file() {
  local file
  for file in CODEOWNERS docs/CODEOWNERS .github/CODEOWNERS; do
    if [[ -f $file ]]; then
      echo "$file"
      return 0
    fi
  done
  return 1
}

# codeowners_base echoes the ref to compare CODEOWNERS with: the branch
# that origin/HEAD points to (for example, origin/main), or main when
# origin/HEAD is not set.
codeowners_base() {
  git symbolic-ref --quiet --short refs/remotes/origin/HEAD || echo main
}

codeowners_linter() {
  # devbase_cli exits when the command ends, so it runs in a subshell.
  (
    devbase_cli lint codeowners \
      --checks files,duppatterns,syntax,owners \
      --owners-if-changed --base "$(codeowners_base)"
  )
}

linter() {
  local show
  # Why: run_command shows the CODEOWNERS file in place of the extensions.
  # shellcheck disable=SC2034
  show="$(codeowners_file)" || return 0
  run_command "codeowners" codeowners_linter
}

formatter() {
  true # No formatter for this linter
}
