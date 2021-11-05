#!/usr/bin/env bash
# Sets up a CircleCI machine to run a devenv instance

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"

# shellcheck source=../lib/logging.sh
source "$DIR/../lib/logging.sh"

info "Setting up devenv container"
docker run --net=host -v /var/run/docker.sock:/var/run/docker.sock -v "$HOME:$HOME" -v "$(pwd):/host_mnt" \
  --name devenv --entrypoint bash -d gcr.io/outreach-docker/devenv:v1.15.6 -c "exec sleep infinity"

# Create CircleCI user and give it the needed perms
docker exec devenv addgroup -g "$(id -g)" circleci
docker exec devenv adduser -u "$(id -u)" -D -H -G circleci circleci
docker exec devenv addgroup circleci docker
docker exec devenv bash -c "echo 'circleci ALL=(ALL) NOPASSWD:ALL' >/etc/sudoers.d/circleci"
docker exec devenv bash -c "mkdir /go; chown -R circleci:circleci /go"
docker exec devenv chown -R circleci:circleci /usr/local/bin

# Allow the devenv to update itself
info "Updating devenv (if needed)"
docker exec --user circleci devenv bash -c "echo '$OUTREACH_GITHUB_TOKEN' > ~/.outreach/github.token"
docker exec --user circleci devenv bash -c "devenv --force-update-check status; devenv --version"

# Setup the name/email for git
docker exec --user circleci devenv git config --global user.name "CircleCI E2E Test"
docker exec --user circleci devenv git config --global user.email "circleci@outreach.io"

docker exec devenv chown :docker /var/run/docker.sock
