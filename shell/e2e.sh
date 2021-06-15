#!/usr/bin/env bash
# Runs e2e tests for the service in the current directory
DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"

if [[ $CI == "true" ]]; then
  "$DIR/circleci/setup-e2e-deps.sh"
  "$DIR/circleci/setup-vault.sh"
  "$DIR/circleci/setup-e2e.sh"

  # Bootstrap puts this here. We could def make this better.
  # sudo is used here because CI has to do some docker perm hacks
  sudo docker exec --user circleci -w /host_mnt -e CI=true -it -e "VAULT_ADDR=$VAULT_ADDR" devenv bash \
    -c "source .bootstrap/shell/lib/ssh-auth.sh; ./scripts/shell-wrapper.sh gobin.sh 'github.com/getoutreach/devbase/e2e@$(cat $DIR/../.version)'"
else
  exec $("$DIR/gobin.sh" -p "github.com/getoutreach/devbase/e2e@$(cat "$DIR/../.version")")
fi
