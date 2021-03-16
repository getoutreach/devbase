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

# has_feature returns 0 if a value is true
# or 1 if false
has_feature() {
  local feat="$1"

  val=$(yq -r ".[\"$feat\"]" <"$(get_service_yaml)")

  if [[ $val == "true" ]]; then
    return 0
  fi

  return 1
}

get_service_yaml() {
  if [[ -e "service.yaml" ]]; then
    echo "service.yaml"
  else
    echo "$REPODIR/service.yaml"
  fi
}

has_resource() {
  local name="$1"

  # Check if the resource is present
  if [[ -n "$(get_resource_version "$name")" ]]; then
    return 0
  fi

  return 1
}

get_resource_version() {
  local name="$1"

  if [[ "$(yq -r '.resources' <"$(get_service_yaml)")" == "null" ]]; then
    echo ""
  else
    yq -r ".resources[\"$name\"]" <"$(get_service_yaml)"
  fi
}

has_grpc_client() {
  local name="$1"

  if [[ "$(yq -r '.grpcClients' <"$(get_service_yaml)")" == "null" ]]; then
    return 1
  fi

  if [[ -n "$(yq -r ".grpcClients[] | select(. == \"$name\")" <"$(get_service_yaml)")" ]]; then
    return 0
  fi

  return 1
}
