#!/usr/bin/env bash
# Uploads code coverage file to S3 via getoutreach/coverbot
# Note: This is not meant to be called outside of coverage.sh
set -e

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
SHELL_DIR="$DIR/../../.."
LIB_DIR="$SHELL_DIR/lib"

# shellcheck source=../../../lib/bootstrap.sh
source "${LIB_DIR}/bootstrap.sh"

coverage_file="$1"

echo "PR Repo Name: $CIRCLE_PROJECT_REPONAME"
echo "Circle PR Number: $CIRCLE_PULL_REQUEST"

if [ -n "$CIRCLE_PULL_REQUEST" ]; then
  # Extract PR number
  PR_NUMBER=$(echo "$CIRCLE_PULL_REQUEST" | awk -F'/' '{print $NF}')
  echo "Parsed PR Number: $PR_NUMBER"

  exec "$SHELL_DIR/gobin.sh" "github.com/getoutreach/coverbot/cmd/coverbot@jackallard17/uploadCovFile" \
    upload --lang "go" --repo "$CIRCLE_PROJECT_REPONAME" --pr "$PR_NUMBER" "$coverage_file"
fi
