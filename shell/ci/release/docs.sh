#!/usr/bin/env bash
# Trigger a private pkg.go.dev instance
set -e

TAG="$CIRCLE_TAG"
if [[ -z $TAG ]]; then
  # Calculate the psuedo-semver tag, this is used for non-v2 services
  # (things without semantic-release, generally)
  TAG="v0.0.0-$(TZ=UTC git --no-pager show --quiet --abbrev=12 --date='format-local:%Y%m%d%H%M%S' --format='%cd-%h')"
fi

# We need to use the module path to support major versions properly
MODULE_PATH="$(go list -f '{{ "{{" }} .Path {{ "}}" }}' -m)"

# TODO(jaredallard): Move this into box configuration?
URL="https://engdocs.outreach.cloud/fetch/$MODULE_PATH@$TAG"

curl -X POST "$URL"
