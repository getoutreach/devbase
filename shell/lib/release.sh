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

# release_has_changes decides whether merging <head> into <base> produces any
# change, without mutating the working tree or HEAD. Uses git merge-tree
# (requires git >= 2.38).
#
# Exit codes:
#   0  clean merge with a real delta to release
#   1  clean merge, no delta (head is an ancestor of base, or already merged)
#   2  merge conflict, or operational error (bad ref, git < 2.38);
#      a self-contained failure report is written to stderr
#
# Requires bootstrap.sh and version.sh to be sourced.
#
# $1 repo directory (git operations run here)
# $2 base ref
# $3 head ref
release_has_changes() {
  local repo_dir="$1"
  local base="$2"
  local head="$3"

  # Fail-fast preflight: merge-tree --write-tree needs git >= 2.38. Without
  # this, an old-git flag error (exit 129) would be reported as a conflict.
  local git_version
  git_version="$(git -C "$repo_dir" --version | awk '{print $3}')"
  if ! has_minimum_version "2.38.0" "$git_version"; then
    {
      echo "release_has_changes: operational error (not a conflict)"
      echo "  dry-run requires git >= 2.38; this environment has $git_version"
      echo "  bump the CI image to a git >= 2.38 build"
    } >&2
    return 2
  fi

  # Validate both refs resolve. merge-tree exits 1 for a bad ref just as it does
  # for a real conflict, so without this a nonexistent ref would be misreported
  # as a merge conflict.
  local ref
  for ref in "$base" "$head"; do
    if ! git -C "$repo_dir" rev-parse --verify --quiet "$ref^{commit}" >/dev/null; then
      {
        echo "release_has_changes: operational error (not a conflict)"
        echo "  ref does not resolve to a commit: $ref"
      } >&2
      return 2
    fi
  done

  local merge_output merge_rc
  merge_output="$(git -C "$repo_dir" merge-tree --write-tree "$base" "$head" 2>&1)"
  merge_rc=$?

  case "$merge_rc" in
  0)
    # Clean merge. First line of stdout is the written tree OID.
    local result_tree base_tree
    result_tree="$(printf '%s\n' "$merge_output" | head -n1)"
    base_tree="$(git -C "$repo_dir" rev-parse "$base^{tree}")"
    if [[ $result_tree == "$base_tree" ]]; then
      return 1
    fi
    return 0
    ;;
  1)
    _release_conflict_report "$repo_dir" "$base" "$head" "$merge_output" >&2
    return 2
    ;;
  *)
    {
      echo "release_has_changes: operational error (not a conflict) merging $head onto $base"
      echo "  git merge-tree exited $merge_rc"
      echo "$merge_output"
    } >&2
    return 2
    ;;
  esac
}

# _release_conflict_report writes a self-contained conflict diagnostic so the
# CI log alone explains the failure. $4 is the captured merge-tree output.
_release_conflict_report() {
  local repo_dir="$1" base="$2" head="$3" merge_output="$4"
  echo "release dry-run: cannot preview $head onto $base (merge conflict)"
  echo "  head: $(git -C "$repo_dir" log -1 --oneline "$head")"
  echo "  base: $(git -C "$repo_dir" log -1 --oneline "$base")"
  echo "  merge-base: $(git -C "$repo_dir" merge-base "$base" "$head")"
  echo "  merge-tree output:"
  echo "$merge_output"
}

# squash_branch squashes <head> onto <base> as a single commit with <message>.
# Only call after release_has_changes has confirmed a delta (exit 0). Runs
# under the caller's set -e, so any failure aborts loudly.
#
# $1 repo directory (git operations run here)
# $2 base ref
# $3 head ref
# $4 commit message
squash_branch() {
  local repo_dir="$1"
  local base="$2"
  local head="$3"
  local message="$4"

  git -C "$repo_dir" checkout "$base"
  git -C "$repo_dir" merge --squash "$head"
  git -C "$repo_dir" commit -m "$message"
}
