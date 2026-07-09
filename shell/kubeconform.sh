#!/usr/bin/env bash
# This is a wrapper around mise to run kubeconform.
# Useful for using the correct version of kubeconform
# with your editor, with the correct cache.
set -euo pipefail

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"

# shellcheck source=./lib/git_cache.sh
source "$DIR/lib/git_cache.sh"

# shellcheck source=./lib/mise/stub.sh
source "$DIR/lib/mise/stub.sh"

# Do not add default to this list. Verified in v0.8.0, the magic
# "default" value adds the raw.githubusercontent.com URL template for
# kubernetes-json-schema, which is rate limited by GitHub.
schemaLocations=()

# Cache the given git repository URL to avoid using
# raw.githubusercontent.com URL templates, which are rate limited
# by GitHub.
upsert_kubeconform_cache() {
  local gitURL="$1"
  local cacheType="$2"
  local pathTemplate="$3"
  local cacheBasename cacheDir
  cacheBasename="$(basename "gitURL")"

  info "Schema cache: $cacheType" >&2
  cacheDir="$(cache_git_repo "$gitURL" kubeconform)"
  schemaLocations+=("$cacheDir/$pathTemplate")
}

upsert_kubeconform_cache https://github.com/yannh/kubernetes-json-schema Kubernetes \
  "{{ .NormalizedKubernetesVersion }}-standalone{{ .StrictSuffix }}/{{ .ResourceKind }}{{ .KindSuffix }}.json"
upsert_kubeconform_cache https://github.com/datreeio/CRDs-catalog "CRDs catalog" \
  "{{ .Group }}/{{ .ResourceKind }}_{{ .ResourceAPIVersion }}.json"

args=()
for location in "${schemaLocations[@]}"; do
  args+=(-schema-location "$location")
done

mise_exec_tool kubeconform "${args[@]}" "$@"
