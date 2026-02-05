#!/usr/bin/env bash
# Ensure that deployments, if they exist, are able to be rendered by
# kubecfg and are valid Kubernetes manifests.

# Why: Used by the script that calls us
# shellcheck disable=SC2034
extensions=(jsonnet)

appName="${DEVENV_DEPLOY_APPNAME:-$(get_app_name)}"

kubecfg_kubeconform() {
  if [[ ! -f "$(get_repo_directory)/$(deployment_source_path "$appName")/$(deployment_manifest_path "$appName")" ]]; then
    info "No jsonnet to be validated, skipping" >&2
    return 0
  fi

  tempFile=$(mktemp)
  if ! "$DIR/build-jsonnet.sh" show >"$tempFile"; then
    error "Failed to render jsonnet"
    return 1
  fi

  if ! mise_exec_tool kubeconform \
    -schema-location default \
    -ignore-missing-schemas \
    -strict \
    -kubernetes-version "$(get_tool_version kubernetes)" \
    -schema-location 'https://raw.githubusercontent.com/datreeio/CRDs-catalog/main/{{.Group}}/{{.ResourceKind}}_{{.ResourceAPIVersion}}.json' \
    <"$tempFile"; then
    error "Failed to validate generated YAML"
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
