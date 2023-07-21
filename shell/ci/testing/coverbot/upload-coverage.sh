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

# assume coverbot-ci-role for S3 bucket permisions
SAFE_CIRCLE_WORKFLOW_ID=$(echo "${CIRCLE_WORKFLOW_ID}" | tr -d -c '[:alnum:]=,.@')
SAFE_CIRCLE_JOB=$(echo "${CIRCLE_JOB}" | tr -d -c '[:alnum:]=,.@')

# Use the OpenID Connect token to obtain AWS credentials
read -r AWS_ACCESS_KEY_ID AWS_SECRET_ACCESS_KEY AWS_SESSION_TOKEN <<<"$(aws sts assume-role-with-web-identity \
  --role-arn arn:aws:iam::182192988802:role/coverbot-ci-role \
  --role-session-name "CircleCI-${SAFE_CIRCLE_WORKFLOW_ID}-${SAFE_CIRCLE_JOB}" \
  --web-identity-token "${CIRCLE_OIDC_TOKEN}" \
  --duration-seconds 3600 \
  --query 'Credentials.[AccessKeyId,SecretAccessKey,SessionToken]' \
  --output text)"

# Export AWS credentials
export AWS_ACCESS_KEY_ID AWS_SECRET_ACCESS_KEY AWS_SESSION_TOKEN

if [ -n "$CIRCLE_PULL_REQUEST" ]; then
  # Extract PR number
  PR_NUMBER=$(echo "$CIRCLE_PULL_REQUEST" | awk -F'/' '{print $NF}')
  echo "Parsed PR Number: $PR_NUMBER"

  exec "$SHELL_DIR/gobin.sh" "github.com/getoutreach/coverbot/cmd/coverbot@jackallard17/uploadCovFile" \
    upload --lang "go" --repo "$CIRCLE_PROJECT_REPONAME" --pr "$PR_NUMBER" "$coverage_file"
fi
