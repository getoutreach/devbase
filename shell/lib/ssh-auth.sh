#!/usr/bin/env bash
# DEPRECATED: Use below path instead
DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
exec "$DIR/../ci/auth/ssh.sh"
