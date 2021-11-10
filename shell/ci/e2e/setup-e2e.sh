#!/usr/bin/env bash
# Sets up a CircleCI machine to run a devenv instance

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"

# shellcheck source=../../lib/logging.sh
source "$DIR/../../lib/logging.sh"

info "Setting up devenv container"
# Pull the devenv out of the container
docker run --entrypoint bash gcr.io/outreach-docker/devenv:v1.15.6 -c 'cat "$(command -v devenv)"' >devenv
sudo mv devenv /usr/local/bin/devenv
sudo chmod +x /usr/local/bin/devenv
sudo chown circleci:circleci /usr/local/bin/devenv

# Allow the devenv to update itself
info "Updating devenv (if needed)"
devenv --force-update-check status
devenv --version
