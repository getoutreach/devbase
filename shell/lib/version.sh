#!/usr/bin/env bash
# Provides version related helper functions

# parse_version(version)
# Parses a version string into major, minor, and patch numbers. Strips leading
# `v`, ignores pre-release info / build metadata, and parses invalid versions as `0.0.0`.
parse_version() {
  # Remove the leading `v` from the first argument (if present).
  local v="${1#v}"
  # Strip pre-release/build metadata: delete from the first `-` or `+` to the end.
  v="${v%%[-+]*}"
  IFS='.' read -r major minor patch <<<"$v"
  printf '%d %d %d' "${major:-0}" "${minor:-0}" "${patch:-0}"
}

# has_minimum_version(minimum_version, version)
# Checks if the given version is greater than or equal to the minimum required version.
# Versions need to follow semantic versioning format: MAJOR.MINOR.PATCH (with optional
# `v` prefix and pre-release info / build metadata suffix).
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
