#!/usr/bin/env bash
#
# Provides yq(), a bash function wrapping `devbase yq`, for callers
# that invoke it often enough that resolving and running it as a fresh
# shell/yq.sh subprocess every time is real, measured overhead: about
# 230ms of `go run` bookkeeping per call when running from source
# (today's common case for most repos), on top of the devbase binary's
# own ~250ms startup cost. yaml_get_field/yaml_get_array (yaml.sh)
# alone run 15+ times per `make` invocation via bootstrap.sh, so that
# per-call cost adds up.
#
# yq() removes the `go run` overhead by building the devbase binary
# once (cached on disk, keyed by the source tree's current commit)
# instead of on every call, then runs that cached binary directly.
# DEVBASE_YQ_BIN, below, also skips the small resolution step itself on
# repeat calls within one shell -- but most real callers capture yq's
# output via `$(...)` or a pipe, both of which fork a subshell, so an
# assignment to DEVBASE_YQ_BIN made inside one of those calls never
# makes it back to the calling shell. A caller that makes several such
# calls in a row (e.g. box.sh's download_box, devconfig.sh) should call
# yq_resolve_bin once, as a plain statement, before the first of them:
# the forked subshells still can't write DEVBASE_YQ_BIN back out, but
# they do inherit whatever the parent shell already set it to, so the
# resolution work runs once instead of once per call. Either way, the
# disk cache is what actually avoids repeating `go build`/`go run`,
# regardless of subshells; it does not remove the binary's own ~250ms
# startup cost, which is inherent to running the compiled Go program at
# all, however it is invoked.
#
# shell/yq.sh remains as a standalone, one-shot script: for callers
# that cannot source a bash function (root/Makefile's $(YQ)) and for
# any caller that only needs yq once, where this caching has nothing
# to amortize.

_DEVBASE_YQ_LIB_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"

# shellcheck source=../cli.sh
source "${_DEVBASE_YQ_LIB_DIR}/../cli.sh"

# DEVBASE_YQ_BIN caches the resolved devbase binary/command for this
# shell session, so repeated yq calls don't redo version/binary
# resolution, or (for the common "local"/dev case) repeat `go run`'s
# own per-call toolchain overhead.
DEVBASE_YQ_BIN=""

# yq_resolve_bin resolves (once) and caches in DEVBASE_YQ_BIN the
# command yq() should run: a one-time-built binary for the "local"/dev
# version, cached by the source tree's current commit (see
# yq_build_from_source), or the same tagged-release resolution
# devbase_cli itself uses otherwise.
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

# yq_build_from_source builds the devbase binary once, caching it by
# the source tree's current commit (dirty-prefixed when there are
# uncommitted changes, so an in-progress edit is never served a stale
# cached build), and echoes its path. Reuses shell/lib/shell.sh's
# cached_binary_path/get_cached_binary, the same pattern shell/gobin.sh
# uses, instead of devbase_cli's own per-call `go run`.
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

# yq runs devbase yq with the given arguments, resolving (once per
# shell session, on first call) which binary to run.
#
# Unlike shell/cli.sh's devbase_cli, this deliberately does not exec:
# exec replaces the calling process image outright, which is correct
# for shell/yq.sh (a one-shot script with nothing left to do
# afterward) but would terminate whatever script sourced this file the
# first time it called yq, instead of letting it call yq again.
yq() {
  yq_resolve_bin
  "$DEVBASE_YQ_BIN" yq "$@"
}
