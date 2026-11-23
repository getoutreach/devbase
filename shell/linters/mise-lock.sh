#!/usr/bin/env bash
# Ensures devbase's mise lockfiles lock exactly the tool versions their
# configs request, with a url and checksum per platform. That covers the
# env lockfiles devbase ships (mise.<env>.lock) and its own mise.lock. See
# `devbase lint mise-lock`. Only devbase has the env configs in its
# repository root, so this does nothing in other repositories.

# Why: Used by the script that calls us
# shellcheck disable=SC2034
extensions=(toml lock)

# mise_lock_env_configs lists the mise env configs devbase ships. They
# mark the devbase repository.
mise_lock_env_configs=(mise.devbase.toml mise.e2e.toml)

linter() {
  local config
  local found=()

  for config in "${mise_lock_env_configs[@]}"; do
    if [[ -f $config ]]; then
      found+=("$config")
    fi
  done

  # Skip before running the devbase CLI, which repositories other than
  # devbase would have to download.
  if [[ ${#found[@]} -eq 0 ]]; then
    return 0
  fi

  if [[ -f mise.toml ]]; then
    found+=(mise.toml)
  fi

  run_command "mise-lock" "$DIR/cli.sh" lint mise-lock "${found[@]}" || return 1
}

formatter() {
  true # No formatter for this linter
}
