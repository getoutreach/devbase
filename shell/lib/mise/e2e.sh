#!/usr/bin/env bash
# E2E-specific mise helpers.
#
# Mirrors the `devbase_*` trio in mise.sh, operating on `mise.e2e.toml`
# (+ `mise.e2e.lock`) under the `e2e` env. Sourced directly by callers
# that need the E2E tool set (currently shell/circleci/machine.sh and
# shell/ci/env/mise.sh) so that mise.sh's namespace stays clean for
# non-E2E consumers.
#
# Requires `mise.sh` (which provides run_mise, mise_version_compatible,
# get_devbase_directory) and `bootstrap.sh` to already be sourced.

# Runs mise in the context of the repo's devbase directory and e2e env.
e2e_mise() {
  local subcommand="$1"
  shift

  if [[ -z $subcommand ]]; then
    fatal "Running e2e_mise requires at least one argument"
  fi

  run_mise "$subcommand" --cd "$(get_devbase_directory)" --env e2e "$@"
}

# Copies mise.e2e.toml and its lockfile to a user-wide config so that
# shims in CI know what to run, with limited network calls.
e2e_configure_global_tools() {
  local miseConfigDir="${MISE_CONFIG_DIR:-${XDG_CONFIG_HOME:-$HOME/.config}/mise}"
  local miseConfdDir="$miseConfigDir/conf.d"
  local userMiseLock="$miseConfigDir/mise.lock"
  local devbaseDir
  devbaseDir="$(get_devbase_directory)"
  mkdir -p "$miseConfdDir"
  cp "$devbaseDir/mise.e2e.toml" "$miseConfdDir/e2e.toml"
  echo >>"$userMiseLock" # touch the lockfile to prevent errors about it not existing
  cat "$devbaseDir/mise.e2e.lock" >>"$userMiseLock"
}

# Installs E2E specific tools if they're not already installed.
e2e_install_mise_tools() {
  # experimental setting needed for Go backend
  if ! mise_version_compatible "2025.10.11"; then
    mise settings set experimental true
  fi
  e2e_mise install --yes
}
