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

# TODO(malept): feature parity with asdf.sh in the same folder.
if [[ -f "$repoDir"/mise.toml ]]; then
  info_sub "🧑‍🍳 installing tool versions via mise"
  if [[ -z ${ALLOW_MISE_TO_MANAGE_TOOL_VERSIONS:-} ]]; then
    info_sub "🧑‍🍳 ignoring .tool-versions (managed by asdf)"
    MISE_GITHUB_TOKEN="$ghToken" \
      MISE_OVERRIDE_TOOL_VERSIONS_FILENAMES="none" \
      mise install --cd "$repoDir" --yes
  else
    info_sub "🧑‍🍳 allowing mise to manage .tool-versions"
    MISE_GITHUB_TOKEN="$ghToken" mise install --cd "$repoDir" --yes
  fi
fi

MISE_GITHUB_TOKEN="$ghToken" devbase_install_mise_tools
devbase_configure_global_tools

if [[ -n ${BASH_ENV:-} ]]; then
  inject_mise_commands >>"$BASH_ENV"
fi
