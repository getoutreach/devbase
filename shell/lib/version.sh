#!/usr/bin/env bash
# Provides version related helper functions

# Parses a version string (e.g. v1.2.3 or 1.2.3 or v1.2.3-alpha) into major, minor, and patch numbers
parse_version() {
  # Remove the leading `v` from the first argument (if present).
  local v="${1#v}"
  # Strip pre-release/build metadata: delete from the first `-` or `+` to the end.
  v="${v%%[-+]*}"
  IFS='.' read -r major minor patch <<<"$v"
  printf '%d %d %d' "${major:-0}" "${minor:-0}" "${patch:-0}"
}

# Checks if the given version is greater than or equal to the minimum required version
has_minimum_version() {
  local min="$1" ver="$2"
  read -r min_major min_minor min_patch <<<"$(parse_version "$min")"
  read -r ver_major ver_minor ver_patch <<<"$(parse_version "$ver")"

  ((ver_major > min_major)) && return 0
  ((ver_major < min_major)) && return 1
  ((ver_minor > min_minor)) && return 0
  ((ver_minor < min_minor)) && return 1
  ((ver_patch >= min_patch))
}
