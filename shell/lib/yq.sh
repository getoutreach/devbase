#!/usr/bin/env bash
#
# yq_wrapper() wraps `devbase yq`, building the devbase binary once per
# source commit (cached on disk) instead of a fresh `go run` per call --
# real savings, since yaml_get_field/yaml_get_array alone call it 15+
# times per `make` invocation. shell/yq.sh stays as the one-shot script
# for callers that can't source a bash function (root/Makefile) or
# only call it once.

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
# otherwise.
yq_resolve_bin() {
  if [[ -n $DEVBASE_YQ_BIN ]]; then
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

# yq_build_from_source builds devbase once per source commit
# (dirty-prefixed when uncommitted changes exist), caching it via
# shell/lib/shell.sh's cached_binary_path/get_cached_binary (same
# pattern as shell/gobin.sh), and echoes its path.
yq_build_from_source() {
  local tree_root rev dirty cache_key cached
  tree_root="$(devbase_tree_root)"
  rev="$(git -C "$tree_root" rev-parse HEAD)" || fatal "failed to resolve $tree_root's current commit"
  dirty=""
  if [[ -n "$(git -C "$tree_root" status --porcelain)" ]]; then
    dirty="dirty-"
  fi
  cache_key="${dirty}${rev}"

  cached="$(get_cached_binary devbase-src "$cache_key")"
  if [[ -z $cached ]]; then
    cached="$(cached_binary_path devbase-src "$cache_key")"
    go -C "$tree_root" build -o "$cached" ./cmd/devbase ||
      fatal "failed to build devbase from $tree_root"
  fi
  echo "$cached"
}

# yq_wrapper runs devbase yq with the given arguments. Unlike
# devbase_cli, it doesn't exec: that would terminate the caller instead
# of letting it call yq_wrapper again.
yq_wrapper() {
  yq_resolve_bin
  "$DEVBASE_YQ_BIN" yq "$@"
}
