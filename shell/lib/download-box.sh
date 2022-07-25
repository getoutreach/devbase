#!/usr/bin/env bash

# Clones a remote box and integrates it with `~/.outreach/.config/box/box.yaml`

HOMEBOX="${HOME}/.outreach/.config/box/box.yaml"
GITREPO="$(yq -r .storageURL "${HOMEBOX}")"

TMPDIR="$(mktemp -d)"
trap 'rm -rf "${TMPDIR}"' EXIT

git clone "${GITREPO}" "${TMPDIR}" --depth 1
NEW_CONF="$(yq . "${HOMEBOX}" | jq --slurpfile boxconf <(yq -r . "${TMPDIR}/box.yaml") '. * {config: $boxconf[0] }' | yq . --yaml-output)"

echo "${NEW_CONF}" >"$HOMEBOX"
