#!/usr/bin/env bash

DEVBASE_CACHE_DIR="$HOME/.outreach/.cache"

# cache_git_repo <gitURL> [cacheSubdir]
#
# Cache the given git repository URL to avoid using
# raw.githubusercontent.com URLs, which are rate limited by GitHub.
# For network/space reasons, this uses a shallow checkout.
#
# If specified, cacheSubdir namespaces the cached git repo.
#
# Prints out the cache directory path.
#
# Assumes that logging.sh is sourced. (Logs are sent to stderr.)
cache_git_repo() {
  local gitURL="$1"
  local cacheSubdir="$2"
  local cacheDir cacheBasename
  cacheBasename="$(basename "gitURL")"
  if [[ -n $cacheSubdir ]]; then
    cacheDir="$DEVBASE_CACHE_DIR/$cacheSubdir/$cacheBasename"
  else
    cacheDir="$DEVBASE_CACHE_DIR/$cacheBasename"
  fi

  if [[ -d $cacheDir ]]; then
    info_sub "Updating local cache" >&2
    git -C "$cacheDir" fetch --depth 1
    git -C "$cacheDir" reset --hard origin/HEAD
  else
    info_sub "Setting up local cache" >&2
    git clone --depth 1 --single-branch "$gitURL" "$cacheDir"
  fi

  echo "$cacheDir"
}
