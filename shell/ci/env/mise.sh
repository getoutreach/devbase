#!/usr/bin/env bash
# Setup mise for an environment. This can be used in both Docker and Machine executors (CircleCI)
# or other CI platforms with that notion.
set -e

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
LIB_DIR="${DIR}/../../lib"

# shellcheck source=../../lib/bootstrap.sh
source "${LIB_DIR}/bootstrap.sh"

repo="$(get_repo_directory)"

# TODO(malept): feature parity with asdf.sh in the same folder.
if [[ -f "$repo"/mise.toml ]]; then
  mise install --cd "$repo" --yes
fi
