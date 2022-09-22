#!/usr/bin/env bash
# Runs the provided tool in an `asdf exec` under an environment that respects
# the tool versions defined in this project's `.tool-versions` file.
#
# This allows scripts to invoke the "right" version of a tool for the given
# project without having to source `asdf`'s setup scripts or be in any
# particular directory.
#
# The idea here is that `some_project/.bootstrap/shell/exec.sh go` will always
# invoke the right go version for that particular `some_project`.
set -e

SCRIPTS_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"

# shellcheck source=./lib/bootstrap.sh
source "$SCRIPTS_DIR/lib/bootstrap.sh"

TOOLVERSIONS="$(get_repo_directory)/.tool-versions"

while read -r line; do
  tool=$(awk '{ print $1 }' <<<"$line" | tr '[:lower:]-' '[:upper:]_')
  version=$(awk '{ print $2 }' <<<"$line")

  export "ASDF_${tool}_VERSION"="${version}"

  # Strip comments just like `asdf` does:
  # https://github.com/asdf-vm/asdf/blob/711ad991043a1980fa264098f29e78f2ecafd610/lib/utils.bash#L653
  #
  # Reverse the file because asdf (buggily? intentionally?)
  # picks the first version it sees if there are duplicates.
  #
  # Uses the `sed` reverse mechanism described here for portability:
  # https://stackoverflow.com/a/744093
done < <(sed '/^[[:blank:]]*#/d;s/#.*//;s/[[:blank:]]*$//' "${TOOLVERSIONS}" | sed '1!G;h;$!d')

exec asdf exec "$@"
