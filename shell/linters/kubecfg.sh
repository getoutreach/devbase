#!/usr/bin/env bash
# Ensure that deployments, if they exist, are able to be rendered by
# kubecfg and are valid Kubernetes manifests.

# shellcheck source=../lib/bootstrap.sh
source "$DIR/lib/bootstrap.sh"

BUILDJSONNETPATH="$DIR/build-jsonnet.sh"
KUBECONFORM=("$DIR/gobin.sh" github.com/yannh/kubeconform/cmd/kubeconform@v0.6.4)

# Why: Used by the script that calls us
# shellcheck disable=SC2034
extensions=(jsonnet)

appName="${DEVENV_DEPLOY_APPNAME:-$(get_app_name)}"

kubernetesVersion=$(get_tool_version kubernetes)

kubecfg_kubeconform() {
  if [[ ! -f "$(get_repo_directory)/deployments/$appName/$appName.jsonnet" ]]; then
    echo "No jsonnet to be validated, skipping" >&2
    return 0
  fi

  tempFile=$(mktemp)
  if ! "$BUILDJSONNETPATH" show >"$tempFile"; then
    echo "Failed to render jsonnet" >&2
    return 1
  fi

  if ! "${KUBECONFORM[@]}" \
    -schema-location default \
    -ignore-missing-schemas \
    -strict \
    -kubernetes-version "$kubernetesVersion" \
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
