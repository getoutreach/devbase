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

# is_semver(version)
#
# Succeeds if version is MAJOR.MINOR.PATCH with an optional pre-release
# suffix, e.g. `1.68.0` or `1.69.0-rc.1`. A simplified form of the
# semver.org regex: no build metadata, and leading zeros are allowed.
# Does not strip a leading `v`.
is_semver() {
  [[ $1 =~ ^[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$ ]]
}

# normalize_semver trims surrounding whitespace and strips a leading
# `v` or `V` before a digit, so mise receives a plain version.
#
# mise 2026.6.14+ drops the `v` when installing a pre-release tag but
# re-adds it during `mise which`/`mise where`, so the tool resolves as
# not installed.
normalize_semver() {
  local version="$1"
  version="${version#"${version%%[![:space:]]*}"}"
  version="${version%"${version##*[![:space:]]}"}"
  if [[ $version =~ ^[vV][0-9] ]]; then
    version="${version:1}"
  fi
  echo "$version"
}
