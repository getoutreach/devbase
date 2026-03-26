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

# Use strict lockfile mode to avoid GitHub API calls for version resolution.
# All tools resolve from pre-resolved URLs in their lockfile instead of
# hitting api.github.com. Set MISE_LOCKED=0 in CircleCI project env to opt out.
export MISE_LOCKED="${MISE_LOCKED:-1}"

# TODO(malept): feature parity with asdf.sh in the same folder.
if [[ -f "$repoDir"/mise.toml ]]; then
  info_sub "🧑‍🍳 installing tool versions via mise"
  if mise_manages_tool_versions; then
    info_sub "🧑‍🍳 allowing mise to manage .tool-versions"
  else
    info_sub "🧑‍🍳 ignoring .tool-versions (managed by asdf)"
  fi
  MISE_GITHUB_TOKEN="$ghToken" run_mise install --cd "$repoDir" --yes
fi

MISE_GITHUB_TOKEN="$ghToken" devbase_install_mise_tools
devbase_configure_global_tools

if [[ -n ${BASH_ENV:-} ]]; then
  inject_mise_commands >>"$BASH_ENV"
fi
