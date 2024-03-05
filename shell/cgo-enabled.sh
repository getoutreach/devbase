#!/usr/bin/env bash
# This is a small script to determine whether a service manifest has
# explicitly enabled compiling with CGo, and print out the appropriate
# value for the CGO_ENABLED environment variable. Defaults to disabled.

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
YQ="$DIR/yq.sh"

# shellcheck source=./lib/bootstrap.sh
source "$DIR/lib/bootstrap.sh"

if [[ "$("$YQ" -r ".arguments.enableCgo" <"$(get_service_yaml)")" == "true" ]]; then
  echo "1"
else
  echo "0"
fi
