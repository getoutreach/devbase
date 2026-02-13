#!/usr/bin/env bash
# Linters for Bash
SHELLFMTPATH="$DIR/shfmt.sh"
SHELLCHECKPATH="$DIR/shellcheck.sh"

# Why: Used by the script that calls us
# shellcheck disable=SC2034
extensions=(sh bash bats)
shebang_paths=(.mise/tasks)

if [[ -n ${DEVBASE_LINT_SHELL_PATHS:-} ]]; then
  IFS=" " read -r -a extraPaths <<<"$DEVBASE_LINT_SHELL_PATHS"
  shebang_paths+=("${extraPaths[@]}")
fi

find_shell_files() {
  (
    find_files_with_extensions "${extensions[@]}"
    find_files_with_shebang "/usr/bin/env bash" "${shebang_paths[@]}"
  ) | sort | uniq
}

shellcheck_linter() {
  # Wrapper script already sets external sources & script directory
  # source path.
  find_shell_files | xargs -n40 "$SHELLCHECKPATH"
}

shellfmt_linter() {
  # Wrapper script already sets --simplify
  find_shell_files | xargs -n40 "$SHELLFMTPATH" --diff
}

shellfmt_formatter() {
  # Wrapper script already sets --simplify
  find_shell_files | xargs -n40 "$SHELLFMTPATH" --write --list
}

linter() {
  run_command "shellcheck" shellcheck_linter || return 1
  run_command "shellfmt" shellfmt_linter || return 1
}

formatter() {
  run_command "shellfmt" shellfmt_formatter
}
