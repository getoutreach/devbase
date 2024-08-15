#!/usr/bin/env bash

set -eo pipefail

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"

if [[ ! -f "$HOME/.aws/credentials" ]]; then
  # shellcheck source=./aws.sh
  source "$DIR/aws.sh"
fi

for registry in $DOCKER_PUSH_REGISTRIES; do
  if [[ $registry =~ amazonaws.com$ ]]; then
    # Format: $ACCOUNT_ID.dkr.ecr.$REGION.amazonaws.com
    region="$(echo "$registry" | cut -d. -f4)"
    # See: https://docs.aws.amazon.com/AmazonECR/latest/userguide/registry_auth.html#registry-auth-token
    aws ecr get-login-password --region "$region" | docker login --username AWS --password-stdin "$registry"
  fi
done
