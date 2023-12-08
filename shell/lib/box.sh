#!/usr/bin/env bash
# Interact with box configuration

# BOXPATH is the default path that box configuration is stored on disk.
BOXPATH="$HOME/.outreach/.config/box/box.yaml"

# LIB_DIR is the directory that shell script libraries live in.
LIB_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"

# shellcheck source=yaml.sh
source "$LIB_DIR/yaml.sh"

# download_box downloads the box configuration and
# saves it to the box configuration file.
download_box() {
  # Attempt to read the URL from env (this would be a CircleCI context or other setting)
  # otherwise fall back to the stub that's created in circleci/setup.sh. Eventually, we'll
  # want to only support the environment variable path.
  local boxGitRepo="$BOX_REPOSITORY_URL"
  if [[ -z $boxGitRepo ]] && [[ ! -e $BOXPATH ]]; then
    echo "No box repository URL provided, and no box configuration stub found at $BOXPATH to infer it from" >&2
    return 1
  elif [[ -z $boxGitRepo ]]; then
    boxGitRepo="$(yq -r '.storageURL' "$BOXPATH")"
  fi

  # Why: OK with assigning without checking exit code.
  # shellcheck disable=SC2155
  local tempDir="$(mktemp -d)"
  trap 'rm -rf "${TMPDIR}"' EXIT

  git clone -q "${boxGitRepo}" "${tempDir}" --depth 1

  if [[ ! -f "${tempDir}/box.yaml" ]]; then
    echo "Cloning failed, cannot find box.yaml" >&2
    return 1
  fi

  # Stub for the below yq command if it doesn't exist
  mkdir -p "$(dirname "$BOXPATH")"
  if [[ ! -e $BOXPATH ]]; then
    echo "{}" >"$BOXPATH"
  fi

  # Why: OK with assigning without checking exit code.
  # shellcheck disable=SC2155
  local newBox=$(yq . "${BOXPATH}" |
    jq --slurpfile boxconf \
      <(yq -r . "${tempDir}/box.yaml") '. * {config: $boxconf[0] }' |
    yq --yaml-output .)
  echo "${newBox}" >"${BOXPATH}"
}

# get_box_yaml returns the box configuration as a yaml string
get_box_yaml() {
  if [[ ! -e ${BOXPATH} ]]; then
    download_box >&2
  fi
  cat "$BOXPATH"
}

# get_box_field returns the value of the field specified by the
# field name from the box configuration.
#
# $1 - field name
get_box_field() {
  local field="$1"
  yaml_get_field ".config.$field" "$BOXPATH"
}
