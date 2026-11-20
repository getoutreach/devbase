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
# devbase_cached_source_binary is what actually avoids rebuilding. A
# caller making several yq_wrapper calls in a row can call yq_resolve_bin
# once first, as a plain statement, so the forked subshells inherit it
# instead of each re-resolving (see box.sh's download_box).
DEVBASE_YQ_BIN=""

# yq_resolve_bin resolves and caches in DEVBASE_YQ_BIN the binary
# yq_wrapper() should run: built-from-source for "local"/dev (see
# devbase_cached_source_binary), or devbase_cli's own tagged-release
# resolution otherwise. It resolves nothing when yq_wrapper will run gojq
# instead.
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
    DEVBASE_YQ_BIN="$(devbase_cached_source_binary)"
    return
  fi

  DEVBASE_YQ_BIN="$(devbase_cli_binary "$version")"
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
  devbase_run "$DEVBASE_YQ_BIN" yq "$@"
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
  devbase_run "$DEVBASE_YQ_BIN" tomlq "$@"
}
