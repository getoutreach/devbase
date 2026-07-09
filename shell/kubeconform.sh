#!/usr/bin/env bash
# This is a wrapper around mise to run kubeconform.
# Useful for using the correct version of kubeconform
# with your editor, with the correct cache.
set -euo pipefail

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"

# shellcheck source=./lib/mise/stub.sh
source "$DIR/lib/mise/stub.sh"

CACHE_DIR="$HOME/.outreach/.cache"

# Do not add default to this list. Verified in v0.8.0, the magic
# "default" value adds the raw.githubusercontent.com URL template for
# kubernetes-json-schema, which is rate limited by GitHub.
schemaLocations=()

# Cache the given git repository URL to avoid using
# raw.githubusercontent.com URL templates, which are rate limited
# by GitHub.
upsert_cache() {
  local gitURL="$1"
  local cacheType="$2"
  local pathTemplate="$3"
  local cacheDir
  cacheDir="$CACHE_DIR/$(basename "$gitURL")"
  if [[ -d $cacheDir ]]; then
    info "Updating local $cacheType cache for manifest validation" >&2
    git -C "$cacheDir" fetch --depth 1
    git -C "$cacheDir" reset --hard origin/HEAD
  else
    info "Setting up local $cacheType cache for manifest validation" >&2
    git clone --depth 1 --single-branch "$gitURL" "$cacheDir"
  fi
  schemaLocations+=("$cacheDir/$pathTemplate")
}

upsert_cache https://github.com/yannh/kubernetes-json-schema "Kubernetes schema" \
  "{{ .NormalizedKubernetesVersion }}-standalone{{ .StrictSuffix }}/{{ .ResourceKind }}{{ .KindSuffix }}.json"
upsert_cache https://github.com/datreeio/CRDs-catalog "CRDs catalog" \
  "{{ .Group }}/{{ .ResourceKind }}_{{ .ResourceAPIVersion }}.json"

args=()
for location in "${schemaLocations[@]}"; do
  args+=(-schema-location "$location")
done

mise_exec_tool kubeconform "${args[@]}" "$@"
