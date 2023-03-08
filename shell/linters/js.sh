#!/usr/bin/env bash
# Linters for js (node)

# Note: This is called from the perspective of shell/
# shellcheck source=../languages/nodejs.sh
source "$DIR/languages/nodejs.sh"

# Why: Used by the script that calls us
# shellcheck disable=SC2034
extensions=(js)

find_node_projects() {
  find_files_with_extensions "package.json"
}

# for_each_package runs a command for each package found in the repo
for_each_package() {
  local cmd="$1"
  readarray -t packages < <(find_node_projects)

  for package in "${packages[@]}"; do
    local dir
    dir="$(dirname "$package")"
    pushd "$dir" >/dev/null || exit
    yarn_install_if_needed
    "$SHELL" -c "$cmd"
    popd >/dev/null || exit
  done
}

prettier_linter() {
  for_each_package "yarn pretty"
}

eslint_linter() {
  for_each_package "yarn lint"
}

prettier_formatter() {
  for_each_package "yarn pretty-fix"
}

eslint_formatter() {
  for_each_package "yarn lint-fix"
}

linter() {
  run_command "eslint" eslint_linter || return 1
  run_command "prettier" prettier_linter || return 1
}

formatter() {
  run_command "eslint" eslint_formatter || return 1
  run_command "prettier" prettier_formatter || return 1
}
