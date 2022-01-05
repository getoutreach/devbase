#!/usr/bin/env bash
# DEPRECATED: Use make e2e infra instead.
# Waits for infrastructure to be ready

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
LIB_DIR="${DIR}/../../lib"

# shellcheck source=../../lib/bootstrap.sh
source "$LIB_DIR/bootstrap.sh"

wantedResources=$(get_keys "resources")

declare -A resources=(
  [postgres]="5432"
  [mysql]="3306"
  [redis]="6379"
  [kafka]="9092"
  [s3]="9000"
  [temporal]="7233"
  [dyanmo_gte_0_11]="4566"
  [dyanmo_lt_0_11]="4569"
)

# resource functions end

for resource in $wantedResources; do
  echo "⏰ Waiting for $resource to be ready"

  if [[ $resource == "dynamo" ]]; then
    # Special override for versions here
    dynamo_version=$(get_resource_version "$resource")
    v1="$(awk -F '.' '{ print $1 }' <<<"$dynamo_version")"
    v2="$(awk -F '.' '{ print $2 }' <<<"$dynamo_version")"

    # if dynamo version is >=0.11 then use a new port
    if [[ $v1 -ge 0 ]] && [[ $v2 -ge 11 ]]; then
      resource="dyanmo_gte_0_11"
    else
      resource="dyanmo_lt_0_11"
    fi
  fi

  port="${resources[$resource]}"
  if [[ -z $port ]]; then
    continue
  fi

  dockerize -wait tcp://localhost:"$port" -timeout 1m
done
