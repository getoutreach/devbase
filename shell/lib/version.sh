#!/usr/bin/env bash
# Provides version related helper functions

# Parses a version string (e\.g\. v1\.2\.3) into major, minor, and patch numbers
parse_version() {
  local v="${1#v}"
  v="${v%%[-+]*}"
  IFS='.' read -r major minor patch <<<"$v"
  printf '%d %d %d' "${major:-0}" "${minor:-0}" "${patch:-0}"
}

# Checks if the given version is greater than or equal to the minimum required version
has_minimum_version() {
  local min="$1" ver="$2"
  read -r min_major min_minor min_patch <<<"$(parse_version "$min")"
  read -r ver_major ver_minor ver_patch <<<"$(parse_version "$ver")"

  if ((ver_major > min_major)); then
    return 0
  elif ((ver_major < min_major)); then
    return 1
  fi

  if ((ver_minor > min_minor)); then
    return 0
  elif ((ver_minor < min_minor)); then
    return 1
  fi

  if ((ver_patch >= min_patch)); then
    return 0
  else
    return 1
  fi
}
