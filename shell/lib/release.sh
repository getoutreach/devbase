#!/usr/bin/env bash
# Helpers for the CI release flow.

# resolve_release_base_branch echoes the branch a release dry-run should be
# previewed against.
#
# Requires bootstrap.sh to be sourced (for stencil_arg).
#
# $1 repo directory (git operations run here)
# $2 current branch
# $3 default branch (origin/HEAD)
resolve_release_base_branch() {
  local repo_dir="$1"
  local current="$2"
  local default_branch="$3"

  if [[ -n ${RELEASE_BASE_BRANCH:-} ]]; then
    printf "%s" "$RELEASE_BASE_BRANCH"
    return 0
  fi

  local prereleases prereleasesBranch
  prereleases="$(stencil_arg "releaseOptions.enablePrereleases")"
  prereleasesBranch="$(stencil_arg "releaseOptions.prereleasesBranch")"
  # stencil_arg returns the literal string "null" for an unset field.
  if [[ -z $prereleasesBranch || $prereleasesBranch == "null" ]]; then
    prereleasesBranch="main"
  fi

  # A stable promotion branch is created from an RC tag, so it is an ancestor
  # of the prereleases branch. RC and feature branches are ahead of it.
  if [[ $prereleases == "true" ]] &&
    git -C "$repo_dir" merge-base --is-ancestor "$current" "$prereleasesBranch" 2>/dev/null; then
    printf "%s" "release"
    return 0
  fi

  printf "%s" "$default_branch"
}

# release_commit_message echoes the combined commit message for squashing
# <head> onto <base>, in chronological order.
#
# $1 repo directory (git operations run here)
# $2 base ref
# $3 head ref
release_commit_message() {
  local repo_dir="$1"
  local base="$2"
  local head="$3"

  git -C "$repo_dir" log "$base..$head" --reverse --format=%B
}
