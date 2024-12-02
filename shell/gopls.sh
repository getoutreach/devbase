#!/bin/bash

set -e

# Parses the embedding project's `.tool-versions` file to find which is the
# currently active version of gopls, then invokes it explicitly through `asdf`.
#
# The idea here is that `some_project/.bootstrap/shell/gopls.sh` will will
# always point to the "right" go for that particular `some_project`.
#
# If this doesn't work right away, try:
# go install golang.org/x/tools/gopls@latest
# asdf reshim

SCRIPTS_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"

exec "$SCRIPTS_DIR/asdf-exec.sh" go run golang.org/x/tools/gopls@v0.14.2

gopls "$@"
