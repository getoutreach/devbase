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

# The Kubernetes version we validate against. Owned here (not passed in by
# callers) so we can both cache the matching schemas and tell kubeconform
# which version to use.
k8sVersion="$(get_tool_version kubernetes)"

# Do not add default to this list. Verified in v0.8.0, the magic
# "default" value adds the raw.githubusercontent.com URL template for
# kubernetes-json-schema, which is rate limited by GitHub.
schemaLocations=()

# Cache the kubernetes-json-schema repo. Pass the repo root as the schema
# location (no .json template): for a bare directory, kubeconform appends
# v<version>-standalone{-strict}/<kind>.json itself. Sparse-checkout only the
# version directories we need, since the full repo is multiple GB.
info "Schema cache: Kubernetes" >&2
k8sCacheDir="$(cache_git_repo https://github.com/yannh/kubernetes-json-schema kubeconform \
  "v${k8sVersion}-standalone" "v${k8sVersion}-standalone-strict")"
schemaLocations+=("$k8sCacheDir")

# Cache the CRDs catalog. It is small (~20 MB), so clone it in full and use
# its explicit path template.
info "Schema cache: CRDs catalog" >&2
crdCacheDir="$(cache_git_repo https://github.com/datreeio/CRDs-catalog kubeconform)"
schemaLocations+=("$crdCacheDir/{{ .Group }}/{{ .ResourceKind }}_{{ .ResourceAPIVersion }}.json")

args=(-kubernetes-version "$k8sVersion")
for location in "${schemaLocations[@]}"; do
  args+=(-schema-location "$location")
done

mise_exec_tool kubeconform "${args[@]}" "$@"
