#!/usr/bin/env bash
#
# AWS ECR authentication. Assumes that logging.sh is sourced.
#

set -eo pipefail

# ecr_auth authenticates with AWS ECR.
# Arguments:
#   $1: The ECR registry URL.
ecr_auth() {
  registry="$1"
  # Format: $ACCOUNT_ID.dkr.ecr.$REGION.amazonaws.com
  region="$(echo "$registry" | cut -d. -f4)"
  info_sub "Authenticating with AWS ECR in $registry"
  # See: https://docs.aws.amazon.com/AmazonECR/latest/userguide/registry_auth.html#registry-auth-token
  aws ecr get-login-password --region "$region" | docker login --username AWS --password-stdin "$registry"
}

# ensure_ecr_repository ensures that an ECR repository exists.
# Arguments:
#   $1: The ECR repository URL.
ensure_ecr_repository() {
  imageRepository="$1"
  # AWS ECR requires that the repository be created before pushing to it.
  # Repo name is the part after the first / in the URL (i.e., ignores the host)
  ecrRepoName=$(cut --delimiter=/ --fields=2- <<<"$imageRepository")
  # Format: $ACCOUNT_ID.dkr.ecr.$REGION.amazonaws.com
  ecrRegion="$(echo "$imageRepository" | cut --delimiter=. --fields=4)"
  if ! aws ecr --region "$ecrRegion" describe-repositories --repository-names "$ecrRepoName"; then
    info_sub "Creating ECR repository: $imageRepository"
    aws ecr --region "$ecrRegion" create-repository --repository-name "$ecrRepoName"
  fi
}
