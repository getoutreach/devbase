#!/usr/bin/env bash
#
# yq_wrapper() wraps `devbase yq`, and tomlq_wrapper() wraps `devbase
# tomlq`, building the devbase binary once per source commit (cached on
# disk) instead of a fresh `go run` per call -- real savings, since
# yaml_get_field/yaml_get_array alone call it 15+ times per `make`
# invocation. shell/yq.sh stays as the one-shot script for callers that
# can't source a bash function (root/Makefile) or only call it once.

_DEVBASE_YQ_LIB_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"

# shellcheck source=../cli.sh
source "${_DEVBASE_YQ_LIB_DIR}/../cli.sh"

# DEVBASE_YQ_BIN caches the resolved binary for this shell. It's set
# inside yq_resolve_bin, which usually runs in a forked subshell ($(...)
# or a pipe) and so can't write it back to the caller -- but repeated
# calls still only build once, since the disk cache in
# yq_build_from_source is what actually avoids rebuilding. A caller
# making several yq_wrapper calls in a row can call yq_resolve_bin once
# first, as a plain statement, so the forked subshells inherit it
# instead of each re-resolving (see box.sh's download_box).
DEVBASE_YQ_BIN=""

# yq_resolve_bin resolves and caches in DEVBASE_YQ_BIN the binary
# yq_wrapper() should run: built-from-source for "local"/dev (see
# yq_build_from_source), or devbase_cli's own tagged-release resolution
# otherwise. It resolves nothing when yq_wrapper will run gojq instead.
yq_resolve_bin() {
  if [[ -n $DEVBASE_YQ_BIN ]]; then
    return
  fi

  if yq_use_gojq; then
    return
  fi

  local version
  version="$(devbase_cli_version)"

  if ! devbase_cli_is_tagged_version "$version"; then
    DEVBASE_YQ_BIN="$(yq_build_from_source)"
    return
  fi

  DEVBASE_YQ_BIN="$(devbase_cli_binary "$version")"
}

# yq_build_from_source builds devbase once per source tree state,
# caching it via shell/lib/shell.sh's cached_binary_path/get_cached_binary
# (same pattern as shell/gobin.sh), and echoes its path. The cache key is
# the current commit when the tree is clean, or that commit plus a
# content hash of the uncommitted changes (see yq_dirty_tree_hash) when
# it isn't -- two different uncommitted edits at the same commit must
# not collide on a single flat "dirty" key.
yq_build_from_source() {
  local tree_root rev cache_key cached
  tree_root="$(devbase_tree_root)"
  rev="$(git -C "$tree_root" rev-parse HEAD)" || fatal "failed to resolve $tree_root's current commit"
  cache_key="$rev"
  # `git status --porcelain` is a cheap dirty-or-not gate: it's what
  # keeps the common, repeated-per-make-invocation clean-tree path (see
  # this file's own top comment) down to two git calls instead of
  # unconditionally paying for yq_dirty_tree_hash's diff/ls-files/hash
  # work on every call.
  if [[ -n "$(git -C "$tree_root" status --porcelain)" ]]; then
    cache_key="dirty-$(yq_dirty_tree_hash "$tree_root")-${rev}"
  fi

  cached="$(get_cached_binary devbase-src "$cache_key")"
  if [[ -z $cached ]]; then
    cached="$(cached_binary_path devbase-src "$cache_key")"
    devbase_build_from_source "$cached" || exit 1
  fi
  echo "$cached"
}

# yq_dirty_tree_hash hashes tree_root's uncommitted content: tracked
# changes against HEAD (staged or not, including binary diffs) plus the
# contents of untracked, non-ignored files. One `cat` per untracked file
# (rather than batching through `xargs -0 cat --`) is deliberate: a bare
# `cat` xargs runs when its input is empty reads from stdin and can hang
# differently across the GNU (Linux) and BSD (macOS) xargs this script
# must run under, which costs more than the handful of forks it would
# save on this already-rare, already-small dirty-tree path.
yq_dirty_tree_hash() {
  local tree_root="$1"
  {
    git -C "$tree_root" diff HEAD --binary
    git -C "$tree_root" ls-files --others --exclude-standard -z |
      while IFS= read -r -d '' f; do cat "$tree_root/$f"; done
  } | sha256sum | awk '{ print $1 }'
}

# yq_use_gojq returns success if `devbase yq` cannot be built here: the
# devbase version is not a tagged release and Go is not installed. A
# tagged release uses a downloaded binary, which does not need Go.
yq_use_gojq() {
  ! devbase_cli_is_tagged_version "$(devbase_cli_version)" &&
    ! find_tool go >/dev/null 2>&1
}

# yq_run_gojq runs `gojq`, which `devbase yq` replaced, with the given
# arguments. Callers on an unstable devbase version do not use the
# `devbase yq` flags that `gojq` lacks.
yq_run_gojq() {
  local gojq_path
  gojq_path="$(find_tool gojq || true)"
  if [[ -z $gojq_path ]]; then
    fatal "neither go nor gojq found; cannot run yq"
  fi
  "$gojq_path" --yaml-input "$@"
}

# yq_wrapper runs devbase yq with the given arguments, or gojq when Go is
# missing (see yq_use_gojq). Unlike devbase_cli, it doesn't exec: that
# would terminate the caller instead of letting it call yq_wrapper again.
yq_wrapper() {
  if yq_use_gojq; then
    yq_run_gojq "$@"
    return
  fi
  yq_resolve_bin
  "$DEVBASE_YQ_BIN" yq "$@"
}

# tomlq_wrapper runs devbase tomlq with the given arguments, resolving
# the binary the same way yq_wrapper does. It has no gojq fallback, as
# gojq cannot read TOML, so it fails when devbase can be neither built
# nor downloaded (see yq_use_gojq).
tomlq_wrapper() {
  if yq_use_gojq; then
    fatal "go not found and devbase is not a tagged release; cannot run tomlq"
  fi
  yq_resolve_bin
  "$DEVBASE_YQ_BIN" tomlq "$@"
}
