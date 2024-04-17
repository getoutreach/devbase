#!/usr/bin/env bash
#
# Syncs the service catalog manifest for the given repository with
# the metadata present in the repository.

set -euo pipefail

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
# shellcheck source=./lib/logging.sh
source "$DIR/lib/logging.sh"
# shellcheck source=./lib/yaml.sh
source "$DIR/lib/yaml.sh"

sed_replace() {
  local pattern=$1
  local replacement=$2
  local file=$3
  local SED
  case "$OSTYPE" in
  darwin*)
    SED="sed -i ''"
    ;;
  linux*)
    SED="sed -i"
    ;;
  esac
  $SED "s|$pattern|$replacement|g" "$file"
}

sync_cortex() {
  info "Syncing cortex.yaml"
  local golang_version lintroller reporting_team stencil_version
  golang_version="$(grep -w ^golang .tool-versions | awk '{print $2}')"
  lintroller="$(yaml_get_field .arguments.lintroller service.yaml)"
  reporting_team="$(yaml_get_field .arguments.reportingTeam service.yaml)"
  stencil_version="$(yaml_get_field .version stencil.lock)"
  sed_replace '\(golang_version:\) .\+' "\1 $golang_version" cortex.yaml
  sed_replace '\(lintroller:\) .\+' "\1 $lintroller" cortex.yaml
  sed_replace '\(reporting_team:\) .\+' "\1 $reporting_team" cortex.yaml
  sed_replace '\(stencil_version:\) .\+' "\1 $stencil_version" cortex.yaml
}

if [[ -f cortex.yaml ]]; then
  sync_cortex
fi
