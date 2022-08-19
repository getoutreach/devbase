#!/usr/bin/env bash
# Linters for stork.yaml and manifest.yaml

# Why: Used by the script that calls us
# shellcheck disable=SC2034
files=(manifest.yaml)

check_cfg() {
  if ! "$CHECK_CONFIG" >/dev/null 2>&1; then
    error "check_config encountered problems with the stork.yaml and manifest.yaml files which make them incompatible with the Stork API. Run check_config for more info."
    exit 1
  fi
}

linter() {
  CHECK_CONFIG=$("$DIR/gobin.sh" -p github.com/getoutreach/stork/cmd/check_config@v"$(get_tool_version "check_config")")
  run_linter "check_config" check_cfg
}
