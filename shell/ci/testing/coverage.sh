#!/usr/bin/env bash
# Upload coverage information to a supported/configured provider.
DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"

# shellcheck source=../../lib/bootstrap.sh
source "$DIR/../../lib/bootstrap.sh"
# shellcheck source=../../lib/logging.sh
source "$DIR/../../lib/logging.sh"

coverage_provider=$(yq '.arguments.coverage' <"$(get_service_yaml)")

if [[ $coverage_provider == "codecov" ]]; then
  "$DIR/codecov/upload-coverage.sh"
elif [[ $coverage_provider == "coveralls" ]]; then
  "$DIR/coveralls/upload-coverage.sh"
else
  error "Unknown coverage provider \"$coverage_provider\", skipping coverage upload"
  exit 0
fi
