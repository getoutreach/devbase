#!/usr/bin/env bash
#
# Syncs the CircleCI orb definition with the version of devbase in the
# stencil.lock file.  By default, it only updates .circleci/config.yml
# (the default config file), but this is only necessary for repositories
# which do not use stencil-circleci to manage the CircleCI config.
# The default config file is validated, others are not (as they may not
# be config files per se, such as orb definitions).

set -euo pipefail

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
DEVBASE_LIB_DIR="$DIR/lib"

# shellcheck source=lib/box.sh
source "$DEVBASE_LIB_DIR"/box.sh

# shellcheck source=lib/sed.sh
source "$DEVBASE_LIB_DIR"/sed.sh

# stencil_module_version parses the version of the given module
# from stencil.lock.
stencil_module_version() {
  local module_name="$1"
  "$DIR/yq.sh" --raw-output ".modules[] | select(.name == \"$module_name\").version" stencil.lock
}

if [[ $# == 0 ]]; then
  stencilCircleCIVersion="$(stencil_module_version github.com/getoutreach/stencil-circleci)"
  if [[ -n $stencilCircleCIVersion ]]; then
    echo "stencil-circleci is in use, skipping CircleCI orb sync"
    exit 0
  fi
fi

devbaseVersion="$(stencil_module_version github.com/getoutreach/devbase)"

if [[ $devbaseVersion =~ -rc ]]; then
  replaceVersion="dev:${devbaseVersion:1}"
elif [[ $devbaseVersion == local ]]; then
  replaceVersion="dev:first"
else
  replaceVersion="${devbaseVersion:1}"
fi

org="$(get_box_field org)"
skipValidate=
for config in .circleci/config.yml "$@"; do
  sed_replace "$org/shared@.\+" "$org/shared@$replaceVersion" "$config"
  if [[ -z $skipValidate ]]; then
    circleci config validate --org-slug="github/$org" "$config"
  fi
  skipValidate=true
done
