#!/usr/bin/env bash
# Ensures each mise env lockfile that devbase ships (mise.<env>.lock)
# locks exactly the tool versions its mise.<env>.toml requests, with a
# url and checksum per platform. See `devbase lint mise-lock`. Only
# devbase has these files in its repository root, so this does nothing
# in other repositories.

# Why: Used by the script that calls us
# shellcheck disable=SC2034
extensions=(toml lock)

# mise_lock_configs lists the mise env configs whose lockfiles this
# linter checks.
mise_lock_configs=(mise.devbase.toml mise.e2e.toml)

linter() {
  local config
  local found=()

  for config in "${mise_lock_configs[@]}"; do
    if [[ -f $config ]]; then
      found+=("$config")
    fi
  done

  # Skip before running the devbase CLI, which repositories other than
  # devbase would have to download.
  if [[ ${#found[@]} -eq 0 ]]; then
    return 0
  fi

  run_command "mise-lock" "$DIR/cli.sh" lint mise-lock "${found[@]}" || return 1
}

formatter() {
  true # No formatter for this linter
}
