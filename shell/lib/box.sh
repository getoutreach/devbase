#!/usr/bin/env bash
# Interact with box configuration
BOXPATH="$HOME/.outreach/.config/box/box.yaml"

# download_box downloads the box configuration and
# saves it to the box configuration file.
download_box() {
  local boxGitRepo="$(yq -r '.storageURL' "${HOMEBOX}")"
  local tempDir="$(mktemp -d)"
  trap 'rm -rf "${TMPDIR}"' EXIT

  git clone -q "${boxGitRepo}" "${tempDir}" --depth 1
  yq . "${BOXPATH}" |
    jq --slurpfile boxconf \
      <(yq -r . "${tempDir}/box.yaml") '. * {config: $boxconf[0] }' |
    yq . --yaml-output >"${BOXPATH}"
}

# get_box_yaml returns the box configuration as a yaml string
get_box_yaml() {
  if [[ ! -e "${BOXPATH}" ]]; then
    download_box >&2
  fi
  cat "$HOME/.outreach/.config/box/box.yaml"
}

# get_box_field returns the value of the field specified by the
# field name from the box configuration.
# Note: .config is automatically included. Format should start with
# a dot. Example: .devenv.imageRegistry
get_box_field() {
  local field="$1"
  yq -r ".config$field" <"$(get_box_yaml)"
}
