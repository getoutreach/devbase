#!/usr/bin/env bash
# Setup AWS access
set -e

# Only run if set
if [[ -z $AWS_ACCESS_KEY_ID ]] || [[ -z $AWS_SECRET_ACCESS_KEY ]]; then
  echo "Skipped: AWS_ACCESS_KEY_ID or AWS_SECRET_ACCESS_KEY is not set"
  exit 0
fi

mkdir -p "$HOME/.aws"
cat >"$HOME/.aws/credentials" <<EOF
[default]
aws_access_key_id        = $AWS_ACCESS_KEY_ID
aws_secret_access_key    = $AWS_SECRET_ACCESS_KEY
EOF
