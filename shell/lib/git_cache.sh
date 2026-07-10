#!/usr/bin/env bash

DEVBASE_CACHE_DIR="$HOME/.outreach/.cache"

# cache_git_repo <gitURL> [cacheSubdir] [sparsePath...]
#
# Cache the given git repository URL to avoid using
# raw.githubusercontent.com URLs, which are rate limited by GitHub.
# For network/space reasons, this uses a shallow checkout.
#
# If specified, cacheSubdir namespaces the cached git repo.
#
# If one or more sparsePath arguments are given, only those top-level
# paths are materialized (via a blobless, sparse checkout). This avoids
# checking out enormous repositories in full when only a few directories
# are needed.
#
# Prints out the cache directory path.
#
# Assumes that logging.sh is sourced. (Logs are sent to stderr.)
cache_git_repo() {
  local gitURL="$1"
  local cacheSubdir="${2:-}"
  # Drop the two leading positional args so "$@" is just the sparse paths.
  # `shift 2` fails when fewer than two args were passed (e.g. a caller that
  # omits cacheSubdir), so fall back to shifting whatever is present.
  shift 2 || shift $#
  local sparsePaths=("$@")

  # Derive the cache dir name from the repo's last path segment. Normalize a
  # trailing slash and a ".git" suffix first so equivalent URLs map to the
  # same name. Callers caching repos whose last segment could collide must
  # pass distinct cacheSubdir values to disambiguate them.
  local cacheDir cacheBasename normalizedURL="${gitURL%/}"
  cacheBasename="$(basename "${normalizedURL%.git}")"
  if [[ -n $cacheSubdir ]]; then
    cacheDir="$DEVBASE_CACHE_DIR/$cacheSubdir/$cacheBasename"
  else
    cacheDir="$DEVBASE_CACHE_DIR/$cacheBasename"
  fi

  # True if every requested sparse path is present and non-empty. A blobless
  # sparse checkout can leave a path unmaterialized after a failed fetch.
  _sparse_paths_materialized() {
    local path
    for path in "${sparsePaths[@]}"; do
      [[ -n "$(find "$cacheDir/$path" -type f -print -quit 2>/dev/null)" ]] || return 1
    done
  }

  if [[ -d $cacheDir ]] && git -C "$cacheDir" rev-parse --git-dir >/dev/null 2>&1; then
    info_sub "Updating local cache" >&2
    # A usable checkout already exists; tolerate a transient refresh failure.
    if ! { git -C "$cacheDir" fetch --depth 1 &&
      git -C "$cacheDir" reset --hard -q origin/HEAD; }; then
      warn "Could not refresh cache at $cacheDir; using the existing checkout" >&2
    fi
    if [[ ${#sparsePaths[@]} -gt 0 ]]; then
      # Local re-apply; tolerate failure so a blip cannot abort a usable cache.
      if ! git -C "$cacheDir" sparse-checkout set "${sparsePaths[@]}"; then
        warn "Could not update sparse paths at $cacheDir; using the existing checkout" >&2
      fi
      # A cache missing its schemas would let -ignore-missing-schemas pass
      # vacuously; fail loudly instead.
      if ! _sparse_paths_materialized; then
        fatal "Cache at $cacheDir is missing requested paths: ${sparsePaths[*]}"
      fi
    fi
  else
    # A leftover directory that is not a healthy git repo (e.g. an
    # interrupted clone) is treated as a cache miss: remove it and re-clone.
    [[ -d $cacheDir ]] && rm -rf "$cacheDir"
    info_sub "Setting up local cache" >&2
    if [[ ${#sparsePaths[@]} -gt 0 ]]; then
      git clone --depth 1 --single-branch --filter=blob:none --sparse \
        "$gitURL" "$cacheDir"
      git -C "$cacheDir" sparse-checkout set "${sparsePaths[@]}"
    else
      git clone --depth 1 --single-branch "$gitURL" "$cacheDir"
    fi
  fi

  echo "$cacheDir"
}
