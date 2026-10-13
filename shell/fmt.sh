#!/usr/bin/env bash
# Run various formatters for our source code
# Note: This is mostly duplicated from linters.sh, we
# will eventually merge this into a better go-based system.
set -e -o pipefail

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"

# shellcheck source=./lib/bootstrap.sh
source "$DIR/lib/bootstrap.sh"
# shellcheck source=./lib/logging.sh
source "$DIR/lib/logging.sh"
# shellcheck source=./lib/shell.sh
source "$DIR/lib/shell.sh"

# add extra (per project) linters
linters=("$DIR/linters"/*.sh)
if [[ -z $workspaceFolder ]]; then
  workspaceFolder="$(get_repo_directory)"
fi
if [[ -d "$workspaceFolder"/scripts/linters ]]; then
  linters+=("$workspaceFolder/scripts/linters/"*.sh)
fi

info "Running formatters"

started_at="$(get_time_ms)"
for linterScript in "${linters[@]}"; do
  "$DIR/run-formatter.sh" "$linterScript" || exit 1
done
finished_at="$(get_time_ms)"
duration="$((finished_at - started_at))"
info "Formatters took $(format_diff $duration)"
