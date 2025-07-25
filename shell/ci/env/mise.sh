#!/usr/bin/env bash
# Setup mise for an environment. This can be used in both Docker and Machine executors (CircleCI)
# or other CI platforms with that notion.
set -e

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
LIB_DIR="${DIR}/../../lib"

# shellcheck source=../../lib/bootstrap.sh
source "${LIB_DIR}/bootstrap.sh"

# shellcheck source=../../lib/logging.sh
source "${LIB_DIR}/logging.sh"

repo="$(get_repo_directory)"

# TODO(malept): feature parity with asdf.sh in the same folder.
if [[ -f "$repo"/mise.toml ]]; then
  info_sub "🧑‍🍳 installing tool versions via mise"
  if [[ -n $ALLOW_MISE_TO_MANAGE_TOOL_VERSIONS ]]; then
    info_sub "🧑‍🍳 ignoring .tool-versions"
    MISE_OVERRIDE_TOOL_VERSIONS_FILENAMES="none" mise install --cd "$repo" --yes
  else
    mise install --cd "$repo" --yes
  fi
fi
