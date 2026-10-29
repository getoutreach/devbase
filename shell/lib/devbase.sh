#!/usr/bin/env bash
#
# Functions for resolving which version of the devbase CLI the current
# repository requires, and for installing it.

_DEVBASE_LIB_DEVBASE_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"

# shellcheck source=./bootstrap.sh
source "${_DEVBASE_LIB_DEVBASE_DIR}/bootstrap.sh"

# devbase_tree_root echoes the root of the devbase source tree this
# file itself lives in: the real devbase repo root when this
# repository is devbase itself, or a consumer repository's .bootstrap
# clone/symlink of it otherwise. It is computed from DEVBASE_DIR,
# deliberately not via get_devbase_directory (or get_app_name, which it
# calls): both read service.yaml's name field through $YQ, which is
# shell/yq.sh -- and shell/yq.sh's own job, once it delegates to
# devbase_cli (shell/cli.sh), is to call that file. Going through $YQ
# here would recurse into the very yq invocation devbase_cli exists to
# resolve.
devbase_tree_root() {
  echo "$DEVBASE_DIR"
}

# devbase_cli_version echoes the devbase version the current
# repository requires: "local" when devbase_tree_root is the current
# repository's own root (working on devbase itself -- never run
# against a tagged copy of itself while developing it) or its
# .version file says "local" (a consumer repo whose stencil.lock
# points at a local checkout); otherwise the real git tag
# scripts/devbase.sh cloned .bootstrap at.
devbase_cli_version() {
  local tree_root repo_root
  tree_root="$(devbase_tree_root)"
  repo_root="$(get_repo_directory)" # pure directory walk, no $YQ.

  if [[ $tree_root == "$repo_root" ]]; then
    echo "local"
    return
  fi
  cat "$tree_root/.version" 2>/dev/null || echo "local"
}

# devbase_cli_is_tagged_version returns success if version (as returned
# by devbase_cli_version) is a real git tag rather than "local"/dev.
# devbase_cli_version never returns a bare "local" that also matches
# ^v[0-9], so testing the regex alone already covers both cases.
devbase_cli_is_tagged_version() {
  [[ $1 =~ ^v[0-9] ]]
}

# devbase_mise_tool echoes the mise tool (without a version) for
# installing the devbase CLI from a GitHub release. It uses the http
# backend instead of the github one, so installing a pinned version
# downloads the release archive directly rather than going through
# GitHub API requests, and verifies the archive against the release's
# checksums.txt. Append "@<version>", without the "v" prefix.
devbase_mise_tool() {
  local releaseURL="https://github.com/getoutreach/devbase/releases/download/v{{ version }}"
  # shellcheck disable=SC2016 # Why: {{ ... }} are mise templates, not shell syntax.
  echo "http:devbase[url=$releaseURL/devbase_{{ version }}_{{ os(macos=\"darwin\") }}_{{ arch(x64=\"amd64\") }}.tar.gz,checksum_url=$releaseURL/checksums.txt]"
}
