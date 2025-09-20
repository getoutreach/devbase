#!/usr/bin/env bash
#
# Sed helper functions.
# Assumes that logging.sh has been sourced already.

# sed_in_place is a wrapper around sed -i that works on both Linux
# and macOS.
sed_in_place() {
  local SED
  case "$OSTYPE" in
  darwin*)
    local gsed="${MACOS_GNU_SED:-gsed}"
    if command -v "$gsed" >/dev/null; then
      SED="$gsed"
    else
      read -r -d '' errMsg <<EOF
macOS support requires GNU sed. Please install it via your developer setup
tool (or via 'brew install gnu-sed' if there is no setup tool) and try the
command again in a new terminal.
EOF
      error "$errMsg"
      return 1
    fi
    ;;
  linux*)
    SED="sed"
    ;;
  esac

  $SED -i "$@"
}

# sed_replace is a wrapper around in-place sed replace that works on
# both Linux and macOS.
sed_replace() {
  local pattern=$1
  local replacement=$2
  local file=$3
  sed_in_place "s|$pattern|$replacement|g" "$file"
}
