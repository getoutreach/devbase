#!/bin/bash

# Parses the embedding project's `.tool-versions` file to find which is the
# currently active version of Go, then invokes it explicitly through `asdf`.
#
# The idea here is that `some_project/.bootstrap/shell/go.sh` will will always
# point to the "right" go for that particular `some_project`.

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
GOVERSION=$(grep golang "${DIR}/../../.tool-versions" | tail -n 1 | awk '{ print $2 }')

if [ -z "${GOVERSION}" ]; then
  echo "Unable to find Go version for this project" >&2
  exit 1
fi

export ASDF_GOLANG_VERSION=${GOVERSION}
asdf exec go "$@"
