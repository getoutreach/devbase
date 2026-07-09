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

# kubeconform_k8s_sparse_dirs <kubeconform-arg...>
#
# Scan kubeconform-style arguments for -kubernetes-version and print the
# kubernetes-json-schema directories we need cached: the -standalone and
# -standalone-strict variants for that version. Prints nothing if no
# version is present.
#
# kubeconform derives the schema path as
# v<version>-standalone{-strict}/... so those are the only directories we
# need to materialize from the (very large) schema repo.
kubeconform_k8s_sparse_dirs() {
  local version=""
  while [[ $# -gt 0 ]]; do
    case "$1" in
    -kubernetes-version)
      version="${2:-}"
      shift 2 || shift $#
      ;;
    -kubernetes-version=*)
      version="${1#-kubernetes-version=}"
      shift
      ;;
    *)
      shift
      ;;
    esac
  done

  if [[ -n $version ]]; then
    echo "v${version}-standalone"
    echo "v${version}-standalone-strict"
  fi
}

# When sourced (e.g. by tests), only define functions; skip the wrapper body.
# BASH_SOURCE[0] equals $0 only when this file is executed directly.
if [[ ${BASH_SOURCE[0]} != "${0}" ]]; then
  return 0
fi

# Do not add default to this list. Verified in v0.8.0, the magic
# "default" value adds the raw.githubusercontent.com URL template for
# kubernetes-json-schema, which is rate limited by GitHub.
schemaLocations=()

# Cache the kubernetes-json-schema repo. Pass the repo root as the schema
# location (no .json template): for a bare directory, kubeconform appends
# v<version>-standalone{-strict}/<kind>.json itself. Sparse-checkout only the
# version directories we need, since the full repo is multiple GB.
k8sSparseDirs=()
while IFS= read -r dir; do
  k8sSparseDirs+=("$dir")
done < <(kubeconform_k8s_sparse_dirs "$@")

info "Schema cache: Kubernetes" >&2
k8sCacheDir="$(cache_git_repo https://github.com/yannh/kubernetes-json-schema \
  kubeconform "${k8sSparseDirs[@]}")"
schemaLocations+=("$k8sCacheDir")

# Cache the CRDs catalog. It is small (~20 MB), so clone it in full and use
# its explicit path template.
info "Schema cache: CRDs catalog" >&2
crdCacheDir="$(cache_git_repo https://github.com/datreeio/CRDs-catalog kubeconform)"
schemaLocations+=("$crdCacheDir/{{ .Group }}/{{ .ResourceKind }}_{{ .ResourceAPIVersion }}.json")

args=()
for location in "${schemaLocations[@]}"; do
  args+=(-schema-location "$location")
done

mise_exec_tool kubeconform "${args[@]}" "$@"
