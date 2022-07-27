#!/usr/bin/env bash
# Linters for js (node)

# Why: Used by the script that calls us
# shellcheck disable=SC2034
extensions=(js)

find_node_projects() {
  git ls-files "*.package.json"
}

prettier_wrapper() {
  readarray -t packages < <(find_node_projects)
  for package in "${packages[@]}"; do
    pushd "$1" >/dev/null 2>&1 || exit 1
    prettier
    popd >/dev/null 2>&1 || exit 1
  done
}

prettier() {
  yarn_install_if_needed
  yarn pretty
}

eslint_wrapper() {
  readarray -t packages < <(find_node_projects)
  for package in "${packages[@]}"; do
    pushd "$1" >/dev/null 2>&1 || exit 1
    estlint
    popd >/dev/null 2>&1 || exit 1
  done
}

eslint() {
  pushd "$1" >/dev/null 2>&1 || exit 1
  yarn_install_if_needed
  yarn lint
  popd >/dev/null 2>&1 || exit 1
}

linter() {
  run_linter "eslint" eslint_wrapper
  run_linter "prettier" prettier_wrapper
}
