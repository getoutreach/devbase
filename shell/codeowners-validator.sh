#!/usr/bin/env bash
# This is a wrapper around gobin.sh to run codeowners-validator, as it uses
# environment variables for arguments.

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
GOBIN="$DIR/gobin.sh"

# shellcheck source=./lib/logging.sh
source "$DIR/lib/logging.sh"
# shellcheck source=./lib/bootstrap.sh
source "$DIR/lib/bootstrap.sh"

GH_TOKEN="$(cat "$HOME/.outreach/github.token")"
if [[ -z $GH_TOKEN ]]; then
  echo "Failed to read Github personal access token" >&2
  exit 1
fi

CODEOWNERS_CHECKS="syntax,files,duppatterns,owners"

# TODO: use org value instead of hardcoding
OWNER_CHECKER_REPOSITORY="getoutreach/$(get_app_name)" \
REPOSITORY_PATH="$(get_repo_directory)" \
CHECKS="$CODEOWNERS_CHECKS" \
GITHUB_ACCESS_TOKEN="$GH_TOKEN" \
  exec "$GOBIN" "github.com/mszostok/codeowners-validator@v$(get_application_version "codeowners-validator")"
