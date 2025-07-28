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

# shellcheck source=lib/bootstrap.sh
source "$DEVBASE_LIB_DIR"/bootstrap.sh

# shellcheck source=lib/box.sh
source "$DEVBASE_LIB_DIR"/box.sh

# shellcheck source=lib/logging.sh
source "$DEVBASE_LIB_DIR"/logging.sh

# shellcheck source=lib/sed.sh
source "$DEVBASE_LIB_DIR"/sed.sh

repoCircleCIConfig=".circleci/config.yml"

if [[ "$(get_app_name)" == "devbase" ]]; then
  isDevbaseItself=true
else
  isDevbaseItself=false
fi

if [[ $# == 0 ]] && managed_by_stencil "$repoCircleCIConfig" && [[ $isDevbaseItself == "false" ]]; then
  info "CircleCI config is managed by Stencil, skipping manual CircleCI orb sync" >&2
  exit 0
fi

if [[ $isDevbaseItself == "true" ]]; then
  # devbase itself will always use a local version of devbase, so have
  # the CircleCI orb use the latest (pre-)release version.
  devbaseVersion="$(get_app_version)"
else
  devbaseVersion="$(stencil_module_version github.com/getoutreach/devbase)"
fi

if [[ $devbaseVersion =~ -rc ]]; then
  replaceVersion="dev:${devbaseVersion:1}"
elif [[ $devbaseVersion == local ]]; then
  replaceVersion="dev:first"
else
  replaceVersion="${devbaseVersion:1}"
fi

info "Replacing CircleCI shared orb version with $replaceVersion"

org="$(get_box_field org)"
for config in "$repoCircleCIConfig" "$@"; do
  info_sub "Updating $config"
  sed_replace "$org/shared@.\+" "$org/shared@$replaceVersion" "$config"
  if [[ $config == "$repoCircleCIConfig" ]]; then
    circleci config validate --org-slug="github/$org" "$config"
  fi
done
