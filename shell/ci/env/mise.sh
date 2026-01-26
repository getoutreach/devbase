#!/usr/bin/env bash
# Setup mise for an environment. This can be used in both Docker and Machine executors (CircleCI)
# or other CI platforms with that notion.
set -e

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
LIB_DIR="${DIR}/../../lib"

# shellcheck source=../../lib/bootstrap.sh
source "${LIB_DIR}/bootstrap.sh"

# shellcheck source=../../lib/github.sh
source "${LIB_DIR}/github.sh"

# shellcheck source=../../lib/logging.sh
source "${LIB_DIR}/logging.sh"

# shellcheck source=../../lib/mise.sh
source "${LIB_DIR}/mise.sh"

repoDir="$(get_repo_directory)"

mise version --quiet

# inject_mise_into_bash_env injects mise support into the value of BASH_ENV.
# Assumes that BASH_ENV is set.
inject_mise_into_bash_env() {
  # Empty echo ensures that we never append to an existing line.
  echo >>"$BASH_ENV"
  mise activate bash --shims >>"$BASH_ENV"
}

# TODO(malept): feature parity with asdf.sh in the same folder.
if [[ -f "$repoDir"/mise.toml ]]; then
  info_sub "🧑‍🍳 installing tool versions via mise"
  if [[ -z $ALLOW_MISE_TO_MANAGE_TOOL_VERSIONS ]]; then
    info_sub "🧑‍🍳 ignoring .tool-versions (managed by asdf)"
    MISE_OVERRIDE_TOOL_VERSIONS_FILENAMES="none" mise install --cd "$repoDir" --yes
  else
    info_sub "🧑‍🍳 allowing mise to manage .tool-versions"
    mise install --cd "$repoDir" --yes
  fi
fi

devbase_install_mise_tools

if [[ -n $BASH_ENV ]]; then
  inject_mise_into_bash_env
fi
