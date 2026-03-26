#!/usr/bin/env bash
# Setup mise for an environment. This can be used in both Docker and Machine executors (CircleCI)
# or other CI platforms with that notion.
set -euo pipefail

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
LIB_DIR="${DIR}/../../lib"

# shellcheck source=../../lib/bootstrap.sh
source "${LIB_DIR}/bootstrap.sh"

# shellcheck source=../../lib/logging.sh
source "${LIB_DIR}/logging.sh"

# shellcheck source=../../lib/mise.sh
source "${LIB_DIR}/mise.sh"

# shellcheck source=../../lib/shell.sh
source "${LIB_DIR}/shell.sh"

# shellcheck source=../../lib/version.sh
source "${LIB_DIR}/version.sh"

repoDir="$(get_repo_directory)"

mise version --quiet

# inject_mise_command echos mise support commands.
inject_mise_commands() {
  # Empty echo ensures that we never append to an existing line.
  echo
  mise activate bash --shims
}

# Assumes that `gh` has already been set up.
ghToken="$(gh auth token)"

# Isolate from global mise config during tool installation.
# Global tools (from orc setup or pre-built CI image in ~/.config/mise/)
# don't have lockfiles and cause --locked to fail. MISE_CONFIG_DIR redirects
# global config discovery to an empty directory so only project-level and
# devbase-level configs are visible. Restored after install for shim setup.
_mise_original_config_dir="${MISE_CONFIG_DIR:-}"
export MISE_CONFIG_DIR="$(mktemp -d)"

# TODO(malept): feature parity with asdf.sh in the same folder.
if [[ -f "$repoDir"/mise.toml ]]; then
  info_sub "🧑‍🍳 installing tool versions via mise"
  if mise_manages_tool_versions; then
    info_sub "🧑‍🍳 allowing mise to manage .tool-versions"
  else
    info_sub "🧑‍🍳 ignoring .tool-versions (managed by asdf)"
  fi

  # Use --locked when a lockfile exists to prevent GitHub API calls for
  # version resolution. Repos without mise.lock fall back to normal install.
  locked_flag=""
  if [[ -f "$repoDir"/mise.lock ]]; then
    locked_flag="--locked"
  fi
  # shellcheck disable=SC2086
  MISE_GITHUB_TOKEN="$ghToken" run_mise install --cd "$repoDir" $locked_flag --yes
fi

MISE_GITHUB_TOKEN="$ghToken" devbase_install_mise_tools

# Restore global config dir for shim setup.
rm -rf "$MISE_CONFIG_DIR"
if [[ -n "$_mise_original_config_dir" ]]; then
  MISE_CONFIG_DIR="$_mise_original_config_dir"
else
  unset MISE_CONFIG_DIR
fi

devbase_configure_global_tools

if [[ -n ${BASH_ENV:-} ]]; then
  inject_mise_commands >>"$BASH_ENV"
fi
