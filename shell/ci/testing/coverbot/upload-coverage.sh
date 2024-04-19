#!/usr/bin/env bash
# Uploads code coverage file to S3 via getoutreach/coverbot
# Note: This is not meant to be called outside of coverage.sh
set -e

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
SHELL_DIR="$DIR/../../.."
LIB_DIR="$SHELL_DIR/lib"

# shellcheck source=../../../lib/bootstrap.sh
source "$LIB_DIR/bootstrap.sh"

# shellcheck source=../../../lib/box.sh
source "$LIB_DIR/box.sh"

# Check if coverage file arg is empty
if [[ -s $1 ]]; then
  coverage_file="$1"
else
  echo "No coverage file provided."
  exit 0
fi

if [[ -z $CIRCLE_PULL_REQUEST ]]; then
  echo "Not on a pull request, aborting" >&2
  exit 0
fi

# If we are on a PR, continue with uploading coverage file to S3

# Regex to comply with what AWS cli expects for session name input
SAFE_CIRCLE_WORKFLOW_ID=$(tr -d -c '[:alnum:]=,.@' <<<"${CIRCLE_WORKFLOW_ID}")
SAFE_CIRCLE_JOB=$(tr -d -c '[:alnum:]=,.@' <<<"${CIRCLE_JOB}")

# Export AWS credentials
export AWS_ACCESS_KEY_ID AWS_SECRET_ACCESS_KEY AWS_SESSION_TOKEN

# Extract PR number
PR_NUMBER=$(awk -F'/' '{print $NF}' <<<"$CIRCLE_PULL_REQUEST")

# Assume coverbot-ci-role for S3 bucket permisions
read -r AWS_ACCESS_KEY_ID AWS_SECRET_ACCESS_KEY AWS_SESSION_TOKEN <<<"$(aws sts assume-role-with-web-identity \
  --role-arn arn:aws:iam::"$(get_box_field aws.defaultAccountID)":role/coverbot-ci-role \
  --role-session-name "CircleCI-${SAFE_CIRCLE_WORKFLOW_ID}-${SAFE_CIRCLE_JOB}" \
  --web-identity-token "${CIRCLE_OIDC_TOKEN}" \
  --duration-seconds 3600 \
  --query 'Credentials.[AccessKeyId,SecretAccessKey,SessionToken]' \
  --output text)"

exec "$SHELL_DIR/gobin.sh" "github.com/getoutreach/coverbot/cmd/coverbot@v1.0.5" \
  upload --lang "go" --repo "$CIRCLE_PROJECT_REPONAME" --pr "$PR_NUMBER" "$coverage_file"
