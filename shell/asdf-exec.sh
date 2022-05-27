#!/usr/bin/env bash

set -e

# Runs the provided tool in an `asdf exec` under an environment that respects
# the tool versions defined in this project's `.tool-versions` file.
#
# This allows scripts to invoke the "right" version of a tool for the given
# project without having to source `asdf`'s setup scripts or be in any
# particular directory.
#
# The idea here is that `some_project/.bootstrap/shell/exec.sh go` will always
# invoke the right go version for that particular `some_project`.

SCRIPTS_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"

# shellcheck source=./lib/bootstrap.sh
source "$SCRIPTS_DIR/lib/bootstrap.sh"

TOOLVERSIONS="$(get_repo_directory)/.tool-versions"

while read -r line; do
  tool=$(awk '{ print $1 }' <<< "$line" | tr '[:lower:]-' '[:upper:]_')
  version=$(awk '{ print $2 }' <<< "$line")

  export "ASDF_${tool}_VERSION"="${version}"
done < <(grep -v '^#' "${TOOLVERSIONS}")

asdf exec "$@"

