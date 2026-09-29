#!/usr/bin/env bash
# Ensures yarn is the only package manager used for Node.js dependencies
# in nodejs_dirs, the only locations where stencil manages a Node.js
# package.json. Fails if a lockfile from another package manager is
# present, or if yarn.lock is missing.

# Why: Used by the script that calls us
# shellcheck disable=SC2034
extensions=(json yaml lock lockb)

# forbidden_lockfiles lists lockfiles produced by Node.js package managers
# other than yarn.
forbidden_lockfiles=(package-lock.json npm-shrinkwrap.json pnpm-lock.yaml bun.lock bun.lockb)

# nodejs_dirs lists Node.js package directories that must ship a
# committed yarn.lock alongside their package.json, and must not contain
# a lockfile from another package manager: the repo root, and stencil's
# generated gRPC Node.js client.
nodejs_dirs=(. api/clients/node)

lockfile_linter() {
  local dir file
  local found=()

  for dir in "${nodejs_dirs[@]}"; do
    for file in "${forbidden_lockfiles[@]}"; do
      if [[ -f "$dir/$file" ]]; then
        found+=("$dir/$file")
      fi
    done
  done

  if [[ ${#found[@]} -gt 0 ]]; then
    error "Only yarn.lock is supported for Node.js dependencies. Remove the following lockfile(s) and use 'yarn install' instead:"
    printf '%s\n' "${found[@]}" >&2
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
