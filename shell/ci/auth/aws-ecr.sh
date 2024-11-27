#!/usr/bin/env bash
# Configures CircleCI Docker authentication for AWS Elastic Container Registry (ECR).
# Assumes that the AWS CLI is installed and configured,
# and that the `DOCKER_PUSH_REGISTRIES` environment variable is set.

set -eo pipefail

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
LIB_DIR="${DIR}/../../lib"
AUTHN_DIR="${LIB_DIR}/docker/authn"

# shellcheck source=../../lib/logging.sh
source "${LIB_DIR}/logging.sh"

# shellcheck source=../../lib/docker/authn/aws-ecr.sh
source "${AUTHN_DIR}/aws-ecr.sh"

if [[ ! -f "$HOME/.aws/credentials" ]]; then
  # shellcheck source=./aws.sh
  source "$DIR/aws.sh"
fi

for registry in $DOCKER_PUSH_REGISTRIES; do
  if [[ $registry =~ amazonaws.com($|/) ]]; then
    ecr_auth "$registry"
  fi
done
