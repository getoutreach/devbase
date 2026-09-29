#!/usr/bin/env bash
# Prevents lockfiles from Node.js package managers other than yarn (npm,
# pnpm, bun) from being committed, and ensures yarn.lock is committed for
# well-known generated Node.js packages.

# Why: Used by the script that calls us
# shellcheck disable=SC2034
extensions=(json yaml lock lockb)

# forbidden_lockfile_pattern matches lockfiles produced by Node.js package
# managers other than yarn.
forbidden_lockfile_pattern='(^|/)(package-lock\.json|npm-shrinkwrap\.json|pnpm-lock\.yaml|bun\.lock|bun\.lockb)$'

# nodejs_dirs lists Node.js package directories that must ship a
# committed yarn.lock alongside their package.json: the repo root, and
# stencil's generated gRPC Node.js client.
nodejs_dirs=(. api/clients/node)

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

yarn_lock_presence_linter() {
  local dir
  local missing=()

  for dir in "${nodejs_dirs[@]}"; do
    if [[ -f "$dir/package.json" && ! -f "$dir/yarn.lock" ]]; then
      missing+=("$dir/yarn.lock")
    fi
  done

  if [[ ${#missing[@]} -gt 0 ]]; then
    error "Missing yarn.lock for the following Node.js package(s):"
    printf '%s\n' "${missing[@]}" >&2
    return 1
  fi

  return 0
}

linter() {
  run_command "node-lockfile" lockfile_linter || return 1
  run_command "node-yarn-lock-presence" yarn_lock_presence_linter || return 1
}

formatter() {
  true # No formatter for this linter
}
