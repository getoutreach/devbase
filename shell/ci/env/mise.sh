#!/usr/bin/env bash
# Setup mise for an environment. This can be used in both Docker and Machine executors (CircleCI)
# or other CI platforms with that notion.
set -euo pipefail

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
LIB_DIR="${DIR}/../../lib"

# shellcheck source=../../lib/bootstrap.sh
source "${LIB_DIR}/bootstrap.sh"

# shellcheck source=../../lib/circleci.sh
source "${LIB_DIR}/circleci.sh"

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
  if mise_manages_tool_versions; then
    info_sub "🧑‍🍳 allowing mise to manage .tool-versions"
  else
    info_sub "🧑‍🍳 ignoring .tool-versions (managed by asdf)"
  fi
  if circleci_should_install_e2e_tools; then
    info_sub "🧑‍🍳 E2E mode: installing only go, node, and tools pinned in mise.e2e.toml"
    export MISE_GITHUB_TOKEN="$ghToken"
    if [[ -f "$repoDir/.tool-versions" ]]; then
      goVersion="$(awk '$1 == "golang" {print $2}' "$repoDir/.tool-versions")"
      nodeVersion="$(awk '$1 == "nodejs" {print $2}' "$repoDir/.tool-versions")"
      if [[ -z $goVersion ]]; then
        fatal "golang version not found in $repoDir/.tool-versions"
      fi
      if [[ -z $nodeVersion ]]; then
        fatal "nodejs version not found in $repoDir/.tool-versions"
      fi
      install_tool_with_mise go "$goVersion"
      install_tool_with_mise node "$nodeVersion"
    else
      install_tool_with_mise go
      install_tool_with_mise node
    fi
    mise_install_tools_for_env e2e
    mise_configure_global_tools_for_env e2e
  else
    MISE_GITHUB_TOKEN="$ghToken" run_mise install --cd "$repoDir" --yes
  fi
fi

if ! circleci_should_install_e2e_tools; then
  MISE_GITHUB_TOKEN="$ghToken" devbase_install_mise_tools
  devbase_configure_global_tools
fi

if [[ -n ${BASH_ENV:-} ]]; then
  inject_mise_commands >>"$BASH_ENV"
fi
