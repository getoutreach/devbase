#!/usr/bin/env bash
# Upload coverage information to a supported/configured provider.
DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"

# shellcheck source=../../lib/bootstrap.sh
source "$DIR/../../lib/bootstrap.sh"
# shellcheck source=../../lib/logging.sh
source "$DIR/../../lib/logging.sh"
# shellcheck source=../../lib/yaml.sh
source "$DIR/../../lib/yaml.sh"

# show_help shows the help message for this script
show_help() {
  echo "Usage: $0 <coverage_file> [group]" >&2
  echo ""
  echo "  coverage_file: The path to the coverage file to upload."
  echo "  group:         Group that this coverage is for, e.g. 'e2e'."
  echo "                 Optional, defaults to coverage providers default test group."
  exit 0
}

# If the arguments is < 1 or > 2, show the help message.
if [[ $# -lt 1 ]] || [[ $# -gt 2 ]]; then
  show_help
fi

file="$1"
group="$2"
if [[ -z $file ]]; then
  show_help
fi

coverage_provider=$(yaml_get_field ".arguments.coverage.provider" "$(get_service_yaml)")
if [[ -z $coverage_provider ]]; then
  info "No coverage provider configured (.arguments.coverage) is empty"
  exit 0
fi

case $coverage_provider in
"coverbot")
  exec "$DIR/coverbot/upload-coverage.sh" "$file" "$group"
  ;;
"codecov")
  exec "$DIR/codecov/upload-coverage.sh" "$file" "$group"
  ;;
"coveralls")
  exec "$DIR/coveralls/upload-coverage.sh" "$file" "$group"
  ;;
*)
  error "Unknown coverage provider \"$coverage_provider\", skipping coverage upload"
  exit 0
  ;;
esac
