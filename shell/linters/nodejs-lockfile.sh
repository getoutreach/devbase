#!/usr/bin/env bash
# Ensures required_lockfile is the only package manager lockfile used
# for Node.js dependencies in nodejs_dirs, the only locations where
# stencil manages a Node.js package.json. Fails if a lockfile from
# another package manager is present, or if required_lockfile is
# missing.

# Why: Used by the script that calls us
# shellcheck disable=SC2034
extensions=(json yaml lock lockb)

# all_lockfiles lists the lockfile produced by every Node.js package
# manager this linter knows about. required_lockfile is the one
# nodejs_dirs must commit; every other entry is forbidden. Changing the
# required package manager only means updating required_lockfile.
all_lockfiles=(yarn.lock package-lock.json npm-shrinkwrap.json pnpm-lock.yaml bun.lock bun.lockb)
required_lockfile=yarn.lock

# nodejs_dirs lists Node.js package directories that must ship a
# committed copy of required_lockfile alongside their package.json, and
# must not contain a lockfile from another package manager: the repo
# root, and stencil's generated gRPC Node.js client.
nodejs_dirs=(. api/clients/node)

lockfile_linter() {
  local dir file
  local found=()

  for dir in "${nodejs_dirs[@]}"; do
    for file in "${all_lockfiles[@]}"; do
      [[ $file == "$required_lockfile" ]] && continue
      if [[ -f "$dir/$file" ]]; then
        found+=("$dir/$file")
      fi
    done
  done

  if [[ ${#found[@]} -gt 0 ]]; then
    error "Only $required_lockfile is supported for Node.js dependencies. Remove the following lockfile(s):"
    printf '%s\n' "${found[@]}" >&2
    return 1
  fi

  return 0
}

required_lockfile_presence_linter() {
  local dir
  local missing=()

  for dir in "${nodejs_dirs[@]}"; do
    if [[ -f "$dir/package.json" && ! -f "$dir/$required_lockfile" ]]; then
      missing+=("$dir/$required_lockfile")
    fi
  done

  if [[ ${#missing[@]} -gt 0 ]]; then
    error "Missing $required_lockfile for the following Node.js package(s):"
    printf '%s\n' "${missing[@]}" >&2
    return 1
  fi

  return 0
}

linter() {
  run_command "nodejs-lockfile" lockfile_linter || return 1
  run_command "node-lockfile-presence" required_lockfile_presence_linter || return 1
}

formatter() {
  true # No formatter for this linter
}
