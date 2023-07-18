#!/usr/bin/env bash
# Ensure that deployments, if they exist, are able to be rendered by
# kubecfg and are valid Kubernetes manifests.

BUILDJSONNETPATH="$DIR/build-jsonnet.sh"
KUBECONFORM=("$DIR/gobin.sh" github.com/yannh/kubeconform/cmd/kubeconform@v0.6.3)

# Why: Used by the script that calls us
# shellcheck disable=SC2034
extensions=(jsonnet)

kubecfg_kubeconform() {
  tempFile=$(mktemp)
  if ! "$BUILDJSONNETPATH" show >"$tempFile"; then
    echo "Failed to render jsonnet" >&2
    return 1
  fi

  if ! "${KUBECONFORM[@]}" \
    -schema-location default \
    -ignore-missing-schemas \
    -strict \
    -kubernetes-version 1.24.15 \
    -schema-location 'https://raw.githubusercontent.com/datreeio/CRDs-catalog/main/{{.Group}}/{{.ResourceKind}}_{{.ResourceAPIVersion}}.json' \
    <"$tempFile"; then
    echo "Failed to validate generated yaml" >&2
    return 1
  fi
}

linter() {
  run_command "kubecfg" kubecfg_kubeconform
}

# formatter is a stub, since this doesn't format anything.
formatter() {
  true
}
