#!/usr/bin/env bash
# Provides version related helper functions

parse_version() {
  local v="${1#v}"
  v="${v%%[-+]*}"
  IFS='.' read -r major minor patch <<<"$v"
  printf '%d %d %d' "${major:-0}" "${minor:-0}" "${patch:-0}"
}
