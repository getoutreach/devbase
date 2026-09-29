#!/usr/bin/env bash
# Prevents lockfiles from Node.js package managers other than yarn (npm,
# pnpm, bun) from being committed. yarn.lock is the only supported lockfile.

# Why: Used by the script that calls us
# shellcheck disable=SC2034
extensions=(json yaml lock lockb)

# forbidden_lockfile_pattern matches lockfiles produced by Node.js package
# managers other than yarn.
forbidden_lockfile_pattern='(^|/)(package-lock\.json|npm-shrinkwrap\.json|pnpm-lock\.yaml|bun\.lock|bun\.lockb)$'

lockfile_linter() {
  local found
  found="$(find_files_with_extensions "${extensions[@]}" | grep -E "$forbidden_lockfile_pattern" || true)"

  if [[ -n $found ]]; then
    error "Only yarn.lock is supported for Node.js dependencies. Remove the following lockfile(s) and use 'yarn install' instead:"
    echo "$found" >&2
    return 1
  fi

  return 0
}

linter() {
  run_command "node-lockfile" lockfile_linter || return 1
}

formatter() {
  true # No formatter for this linter
}
