#!/usr/bin/env bash
# Sets up a CircleCI machine to run a devenv instance

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"

# shellcheck source=../lib/logging.sh
source "$DIR/../lib/logging.sh"

info "Setting up box configuration stub"
boxPath="$HOME/.outreach/.config/box/box.yaml"
mkdir -p "$(dirname "$boxPath")"
cat >"$boxPath" <<EOF
lastUpdated: 2021-01-01T00:00:00.0000000Z
storageURL: git@github.com:getoutreach/box
EOF

"$DIR/../lib/docker-authn.sh"

info "Setting up AWS access"
mkdir -p "$HOME/.aws"
cat >"$HOME/.aws/credentials" <<EOF
[default]
aws_access_key_id        = $AWS_ACCESS_KEY_ID
aws_secret_access_key    = $AWS_SECRET_ACCESS_KEY
EOF

info "Setting up devenv container"
docker run --net=host -v /var/run/docker.sock:/var/run/docker.sock -v "$HOME:$HOME" -v "$(pwd):/host_mnt" \
  --name devenv --entrypoint bash -d gcr.io/outreach-docker/devenv:1.1.2 -c "exec sleep infinity"

# Create CircleCI user and give it the needed perms
docker exec devenv addgroup -g "$(id -g)" circleci
docker exec devenv adduser -u "$(id -u)" -D -H -G circleci circleci
docker exec devenv addgroup circleci docker
docker exec devenv bash -c "echo 'circleci ALL=(ALL) NOPASSWD:ALL' >/etc/sudoers.d/circleci"
docker exec devenv bash -c "mkdir /go; chown -R circleci:circleci /go"

# Allow the devenv to update itself
info "Updating devenv (if needed)"
docker exec --user circleci devenv bash -c "echo '$OUTREACH_GITHUB_TOKEN' > ~/.outreach/github.token"
docker exec --user circleci devenv bash -c "devenv status; devenv --version"

# Setup the name/email for git
docker exec --user circleci devenv git config --global user.name "CircleCI E2E Test"
docker exec --user circleci devenv git config --global user.email "circleci@outreach.io"

docker exec devenv chown :docker /var/run/docker.sock
