#!/usr/bin/env bash
# Get bootstrap information

REPODIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." >/dev/null 2>&1 && pwd)"
BOOTSTRAPDIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." >/dev/null 2>&1 && pwd)"

get_app_name() {
  yq -r '.name' <"$(get_service_yaml)"
}

get_application_version() {
  name="$1"

  yq -r ".[\"$name\"]" <"$BOOTSTRAPDIR/versions.yaml"
}

get_service_yaml() {
  if [[ -e "service.yaml" ]]; then
    echo "service.yaml"
  else
    echo "$REPODIR/service.yaml"
  fi
}

has_resource() {
  name="$1"

  # Check if the resource is present
  if [[ -n "$(get_resource_version "$name")" ]]; then
    return 0
  fi

  return 1
}

get_resource_version() {
  name="$1"

  yq -r ".resources[\"$name\"]" <"$(get_service_yaml)"
}

has_grpc_client() {
  name="$1"

  if [[ -n "$(yq -r ".grpcClients[] | select(. == \"$name\")" <"$(get_service_yaml)")" ]]; then
    return 0
  fi

  return 1
}
