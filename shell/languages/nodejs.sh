#!/usr/bin/env bash
# Helpers for node.js

# shellcheck source=../lib/shell.sh
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)/../lib/shell.sh"

yarn_install_if_needed() {
  local stateFile="node_modules/devbase.lock"

  if ! yarn -v >/dev/null 2>&1; then
    npm install -g yarn
  fi

  if [[ ! -e "node_modules" ]]; then
    yarn_install "$stateFile"
    return
  fi

  if [[ ! -e $stateFile ]]; then
    yarn_install "$stateFile"
    return
  fi

  if [[ "$(cat "$stateFile")" != "$(generate_state_file)" ]]; then
    yarn_install "$stateFile"
    return
  fi

  # should be up-to-date, if not we've done the best we can at this
  # point. User should take over.
  return
}

yarn_install() {
  local stateFile="$1"
  # retry w/ 5s delay, 5 times
  retry 5 5 yarn install
  generate_state_file >"$stateFile"
}

hash_file() {
  local file="$1"
  sha256sum "$file" | awk '{ print $1 }'
}

generate_state_file() {
  echo "$(hash_file "package.json")-$(hash_file "yarn.lock")"
}
