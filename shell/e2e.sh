#!/usr/bin/env bash
# Runs e2e tests for the service in the current directory
DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"

if [[ $CI == "true" ]]; then
  "$DIR/circleci/setup-vault.sh"
  "$DIR/circleci/setup-e2e.sh"

  # Install some dependencies
  curl -fsSL https://apt.releases.hashicorp.com/gpg | sudo apt-key add -
  sudo apt-add-repository "deb [arch=amd64] https://apt.releases.hashicorp.com $(lsb_release -cs) main"
  sudo apt-get update -y && sudo apt-get install -y vault

  # Bootstrap puts this here. We could def make this better.
  # sudo is used here because CI has to do some docker perm hacks
  sudo docker exec -it devenv /host_mnt/scripts/shell-wrapper.sh gobin.sh "github.com/getoutreach/devbase/e2e@$(cat "$DIR/../.version")"
fi

exec $("$DIR/gobin.sh" -p "github.com/getoutreach/devbase/e2e@$(cat "$DIR/../.version")")
