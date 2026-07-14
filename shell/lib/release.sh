#!/usr/bin/env bash
# Helpers for the CI release flow.

# resolve_release_base_branch <repo_dir> <current_branch> <default_branch>
#
# Echoes the branch a release dry-run should be previewed against.
# Requires bootstrap.sh to be sourced (for stencil_arg). <default_branch> is
# origin/HEAD; git operations run in <repo_dir>.
resolve_release_base_branch() {
  local repo_dir="$1"
  local current="$2"
  local default_branch="$3"

  if [[ -n ${RELEASE_BASE_BRANCH:-} ]]; then
    if ! git check-ref-format --branch "$RELEASE_BASE_BRANCH" >/dev/null 2>&1; then
      echo "resolve_release_base_branch: invalid RELEASE_BASE_BRANCH: $RELEASE_BASE_BRANCH" >&2
      return 1
    fi
    printf "%s" "$RELEASE_BASE_BRANCH"
    return 0
  fi

  # Branch-name conventions this helper assumes. Override the resolved
  # base entirely via RELEASE_BASE_BRANCH.
  local -r DEFAULT_PRERELEASES_BRANCH="main"
  local -r STABLE_RELEASE_BRANCH="release"

  local prereleases prereleasesBranch rc
  prereleases="$(stencil_arg "releaseOptions.enablePrereleases")" || rc=$?
  if [[ ${rc:-0} -ne 0 ]]; then
    echo "resolve_release_base_branch: failed to read releaseOptions.enablePrereleases" >&2
    return 1
  fi
  prereleasesBranch="$(stencil_arg "releaseOptions.prereleasesBranch")" || rc=$?
  if [[ ${rc:-0} -ne 0 ]]; then
    echo "resolve_release_base_branch: failed to read releaseOptions.prereleasesBranch" >&2
    return 1
  fi
  # stencil_arg returns the literal string "null" for an unset field.
  if [[ -z $prereleasesBranch || $prereleasesBranch == "null" ]]; then
    prereleasesBranch="$DEFAULT_PRERELEASES_BRANCH"
  fi
  # Reject an invalid branch name before it is used as a git ref below.
  if ! git check-ref-format --branch "$prereleasesBranch" >/dev/null 2>&1; then
    echo "resolve_release_base_branch: invalid prereleasesBranch: $prereleasesBranch" >&2
    return 1
  fi

  # Resolve the prereleases branch to a ref that exists locally. In a
  # single-branch CI clone only the remote-tracking ref may be present.
  local prereleasesRef="$prereleasesBranch"
  if ! git -C "$repo_dir" rev-parse --verify --quiet "$prereleasesBranch^{commit}" >/dev/null &&
    git -C "$repo_dir" rev-parse --verify --quiet "origin/$prereleasesBranch^{commit}" >/dev/null; then
    prereleasesRef="origin/$prereleasesBranch"
  fi

  # A stable promotion branch is created from an RC tag, so it is an ancestor
  # of the prereleases branch. RC and feature branches are ahead of it.
  if [[ $prereleases == "true" ]] &&
    git -C "$repo_dir" merge-base --is-ancestor "$current" "$prereleasesRef" 2>/dev/null; then
    printf "%s" "$STABLE_RELEASE_BRANCH"
    return 0
  fi

  printf "%s" "$default_branch"
}

# release_commit_message <repo_dir> <base> <head>
#
# Echoes the combined commit message for squashing <head> onto <base>, in
# chronological order. Git operations run in <repo_dir>.
release_commit_message() {
  local repo_dir="$1"
  local base="$2"
  local head="$3"

  git -C "$repo_dir" log "$base..$head" --reverse --format=%B
}

# release_has_changes <repo_dir> <base> <head>
#
# Decides whether merging <head> into <base> produces any change, without
# mutating the working tree or HEAD. Uses git merge-tree (requires git >= 2.38).
# Requires bootstrap.sh and version.sh to be sourced. Git operations run in
# <repo_dir>.
#
# Exit codes:
#   0  clean merge with a real delta to release
#   1  clean merge, no delta (head is an ancestor of base, or already merged)
#   2  merge conflict, or operational error (bad ref, git < 2.38);
#      a self-contained failure report is written to stderr
release_has_changes() {
  local repo_dir="$1"
  local base="$2"
  local head="$3"

  # Fail-fast preflight: without it, old git's flag error (exit 129) would be
  # misreported as a conflict.
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
    _release_conflict_report "$repo_dir" "$base" "$head" "$merge_output"
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

# _release_conflict_report <repo_dir> <base> <head> <merge_output>
#
# Writes a self-contained conflict diagnostic to stderr so the CI log alone
# explains the failure. <merge_output> is the captured merge-tree output.
_release_conflict_report() {
  local repo_dir="$1" base="$2" head="$3" merge_output="$4"
  {
    echo "release dry-run: cannot preview $head onto $base (merge conflict)"
    echo "  head: $(git -C "$repo_dir" log -1 --oneline "$head")"
    echo "  base: $(git -C "$repo_dir" log -1 --oneline "$base")"
    echo "  merge-base: $(git -C "$repo_dir" merge-base "$base" "$head")"
    echo "  merge-tree output:"
    echo "$merge_output"
  } >&2
}

# squash_branch <repo_dir> <base> <head> <message>
#
# Squashes <head> onto <base> as a single commit with <message>. Only call
# after release_has_changes has confirmed a delta (exit 0). Runs under the
# caller's set -e, so any failure aborts loudly. On success the working tree is
# left checked out on <base> (the squash commit is on <base>).
squash_branch() {
  local repo_dir="$1"
  local base="$2"
  local head="$3"
  local message="$4"

  git -C "$repo_dir" checkout "$base"
  git -C "$repo_dir" merge --squash "$head"
  git -C "$repo_dir" commit -m "$message"
}
