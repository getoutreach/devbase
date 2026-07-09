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
  local cacheSubdir="$2"
  shift 2 || shift $#
  local sparsePaths=("$@")

  local cacheDir cacheBasename
  cacheBasename="$(basename "$gitURL")"
  if [[ -n $cacheSubdir ]]; then
    cacheDir="$DEVBASE_CACHE_DIR/$cacheSubdir/$cacheBasename"
  else
    cacheDir="$DEVBASE_CACHE_DIR/$cacheBasename"
  fi

  if [[ -d $cacheDir ]]; then
    info_sub "Updating local cache" >&2
    git -C "$cacheDir" fetch --depth 1
    git -C "$cacheDir" reset --hard origin/HEAD
    if [[ ${#sparsePaths[@]} -gt 0 ]]; then
      git -C "$cacheDir" sparse-checkout set "${sparsePaths[@]}"
    fi
  else
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
