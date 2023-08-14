#!/usr/bin/env bash
# This contains a linter framework for running
# linters.
set -e -o pipefail

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"

# shellcheck source=./lib/logging.sh
source "$DIR/lib/logging.sh"
# shellcheck source=./lib/bootstrap.sh
source "$DIR/lib/bootstrap.sh"
# shellcheck source=./lib/shell.sh
source "$DIR/lib/shell.sh"

if [[ -n $SKIP_LINTERS ]] || [[ -n $SKIP_VALIDATE ]]; then
  info "Skipping linters"
  exit 0
fi

# add extra (per project) linters
linters=("$DIR/linters"/*.sh)
if [[ -z $workspaceFolder ]]; then
  workspaceFolder="$(get_repo_directory)"
fi
if [[ -d "$workspaceFolder"/scripts/linters ]]; then
  linters+=("$workspaceFolder/scripts/linters/"*.sh)
fi

info "Running linters"

started_at="$(get_time_ms)"
for linterScript in "${linters[@]}"; do
  # We use a sub-shell to prevent inheriting
  # the changes to functions/variables to the parent
  # (this) script
  (
    # Note: These are modified by the source'd linter file
    # extensions are the extensions this linter should run on
    extensions=()

    # Why: Dynamic
    # shellcheck disable=SC1090
    source "$linterScript"

    # If we don't find any files with the extension, skip the run.
    matched=false
    if [[ "$(find_files_with_extensions "${extensions[@]}" | wc -l | tr -d ' ')" -gt 0 ]]; then
      matched=true
    fi

    if [[ $matched == "false" ]]; then
      exit 0
    fi

    # Note: extensions is set by the linter.
    # Why: We're OK with declaring and assigning.
    # shellcheck disable=SC2155,SC2001
    extensionsString=$(sed 's/ /,./g' <<<"${extensions[*]}" | sed 's/^/./')

    # show is used by run_command as metadata to be shown along with the command name
    show=$extensionsString

    # Set by the language file
    if ! linter; then
      error "linter failed to run. Please check the logs, 'make fmt' may help in some cases."
      exit 1
    fi
  )
done
finished_at="$(get_time_ms)"
duration="$((finished_at - started_at))"
info "Linters took $(format_diff $duration)"
