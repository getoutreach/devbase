#!/bin/bash

set -e

# Parses the embedding project's `.tool-versions` file to find which is the
# currently active version of Go, then invokes it explicitly through `asdf`.
#
# The idea here is that `some_project/.bootstrap/shell/go.sh` will will always
# point to the "right" go for that particular `some_project`.

SCRIPTS_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"

exec "$SCRIPTS_DIR/asdf-exec.sh" go "$@"
