#!/usr/bin/env bash
# This is a wrapper around gobin.sh to run terraform.
# Useful for using the correct version of terraform with your editor.

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
GOBIN="$DIR/gobin.sh"

# shellcheck source=./lib/bootstrap.sh
source "$DIR/lib/bootstrap.sh"

exec "$GOBIN" "github.com/hashicorp/terraform@v$(get_application_version "terraform")" "$@"
