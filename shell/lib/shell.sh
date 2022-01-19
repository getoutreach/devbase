#!/usr/bin/env bash
# Provides generic shell helper functions

DEVBASE_CACHED_BINARY_STORAGE_PATH="$HOME/.outreach/.cache/devbase"

# retry calls a given command (must be wrapped in quotes)
# syntax: retry <interval> <maxRetries> <command> [args...]
retry() {
  local interval="$1"
  local maxRetries="$2"
  local command="$3"

  # remove interval+command from the argument stack
  # so we can send it to the command later
  shift
  shift
  shift

  local exitCode=0
  for i in $(seq 1 "$maxRetries"); do
    if [[ $i -gt 1 ]]; then
      echo "RETRYING: $command ($i/$maxRetries)"
    fi

    # execute command, if succeeds break out of loop and
    # and reset exitCode
    "$command" "$@" && exitCode=0 && break

    # preserve the exit code
    exitCode=$?

    # try again after x time
    sleep "$interval"
  done

  return $exitCode
}

# cached_binary_path returns the raw path of a binary if it
# were to be cached. It is not guaranteed to exist.
cached_binary_path() {
  local name="$1"
  local version="$2"

  echo "$DEVBASE_CACHED_BINARY_STORAGE_PATH/$name/$version/$name"
}

# get_cached_binary returns the path to a cached binary
# or returns empty if not found
get_cached_binary() {
  local name="$1"
  local version="$2"

  # shellcheck disable=SC2155 # Why: No return value
  local cachedPath=$(cached_binary_path "$name" "$version")

  # Create the base path.
  mkdir -p "$(dirname "$cachedPath")"

  if [[ ! -e $cachedPath ]]; then
    echo ""
  else
    echo "$cachedPath"
  fi
}
