#!/usr/bin/env bash
# Resolves the branch a release dry-run should be previewed against.

BASE_BRANCH_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"

# shellcheck source=../../lib/yaml.sh
source "${BASE_BRANCH_DIR}/../../lib/yaml.sh"

# resolve_release_base_branch echoes the branch to preview a release against.
#
# $1 repo directory (git operations run here)
# $2 current branch
# $3 default branch (origin/HEAD)
# $4 path to service.yaml
resolve_release_base_branch() {
  local repo_dir="$1"
  local current="$2"
  local default_branch="$3"
  local service_yaml="$4"

  if [[ -n ${RELEASE_BASE_BRANCH:-} ]]; then
    printf "%s" "$RELEASE_BASE_BRANCH"
    return 0
  fi

  local prereleases prereleasesBranch
  prereleases="$(yaml_get_field ".arguments.releaseOptions.enablePrereleases" "$service_yaml")"
  prereleasesBranch="$(yaml_get_field ".arguments.releaseOptions.prereleasesBranch" "$service_yaml")"
  prereleasesBranch="${prereleasesBranch:-main}"

  # A stable promotion branch is created from an RC tag, so it is an ancestor
  # of the prereleases branch. RC and feature branches are ahead of it.
  if [[ $prereleases == "true" ]] &&
    git -C "$repo_dir" merge-base --is-ancestor "$current" "$prereleasesBranch" 2>/dev/null; then
    printf "%s" "release"
    return 0
  fi

  printf "%s" "$default_branch"
}
