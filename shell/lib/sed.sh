#!/usr/bin/env bash
#
# Sed helper functions.

# sed_in_place is a wrapper around sed -i that works on both Linux
# and macOS.
sed_in_place() {
  local SED
  case "$OSTYPE" in
  darwin*)
    SED="sed -i ''"
    ;;
  linux*)
    SED="sed -i"
    ;;
  esac

  $SED "$@"
}

# sed_replace is a wrapper around in-place sed replace that works on
# both Linux and macOS.
sed_replace() {
  local pattern=$1
  local replacement=$2
  local file=$3
  sed_in_place "s|$pattern|$replacement|g" "$file"
}
