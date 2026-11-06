#!/usr/bin/env bash
# Provides version related helper functions

# parse_version(version)
#
# Parses a semantic version (semver)-compatible string into major,
# minor, and patch numbers. Strips leading `v`, ignores pre-release
# info / build metadata, and parses empty versions as `0.0.0`.
# Partial versions have missing parts parsed as `0`. Fails if any
# version part is not a number.
parse_version() {
  # Remove the leading `v` from the first argument (if present).
  local v="${1#v}"
  # Strip pre-release/build metadata: delete from the first `-` or `+` to the end.
  v="${v%%[-+]*}"
  IFS='.' read -r major minor patch <<<"$v"
  printf '%d %d %d' "${major:-0}" "${minor:-0}" "${patch:-0}"
}

# has_minimum_version(minimum_version, version)
#
# Checks if the given version is greater than or equal to the minimum
# required version. Versions are parsed with parse_version().
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

# is_prerelease_version(version)
#
# Succeeds if the given semantic version (semver) has a pre-release
# suffix (e.g. `v1.2.3-rc.1`). Build metadata (`+...`) alone does not
# make a version a pre-release.
is_prerelease_version() {
  local v="${1%%+*}"
  [[ $v == *-* ]]
}
