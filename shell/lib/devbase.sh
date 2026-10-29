#!/usr/bin/env bash
#
# Helpers for finding the devbase version a repository requires and for
# installing the devbase CLI.

_DEVBASE_LIB_DEVBASE_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"

# shellcheck source=./bootstrap.sh
source "${_DEVBASE_LIB_DEVBASE_DIR}/bootstrap.sh"

# devbase_tree_root echoes the root of the devbase source tree that
# bootstrap.sh belongs to: devbase's own repository, or a consumer
# repository's .bootstrap directory. It does not use
# get_devbase_directory, because that reads service.yaml through $YQ,
# and shell/yq.sh calls devbase_cli, which calls this function.
devbase_tree_root() {
  echo "$DEVBASE_DIR"
}

# devbase_cli_version echoes the devbase version the repository
# requires. It echoes "local" when this repository is devbase itself or
# when .version says "local". Otherwise it echoes the tag in .version,
# which scripts/devbase.sh wrote when it cloned .bootstrap.
devbase_cli_version() {
  local tree_root repo_root
  tree_root="$(devbase_tree_root)"
  repo_root="$(get_repo_directory)"

  if [[ $tree_root == "$repo_root" ]]; then
    echo "local"
    return
  fi
  cat "$tree_root/.version" 2>/dev/null || echo "local"
}

# devbase_cli_is_tagged_version succeeds if the version from
# devbase_cli_version is a release tag rather than "local".
devbase_cli_is_tagged_version() {
  [[ $1 =~ ^v[0-9] ]]
}

# devbase_mise_tool echoes the mise tool for installing the devbase CLI,
# without a version. Append "@<version>" without the "v" prefix. It uses
# the http backend, which downloads the release archive directly
# instead of making GitHub API requests, and checks the archive against
# the release's checksums.txt.
devbase_mise_tool() {
  local releaseURL="https://github.com/getoutreach/devbase/releases/download/v{{ version }}"
  echo "http:devbase[url=$releaseURL/devbase_{{ version }}_{{ os(macos=\"darwin\") }}_{{ arch(x64=\"amd64\") }}.tar.gz,checksum_url=$releaseURL/checksums.txt]"
}
