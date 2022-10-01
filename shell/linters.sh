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

info "Running linters"

started_at="$(get_time_ms)"
for languageScript in "$DIR/linters"/*.sh; do
  languageName="$(basename "${languageScript%.sh}")"

  # We use a sub-shell to prevent inheriting
  # the changes to functions/variables to the parent
  # (this) script
  (
    # Note: These are modified by the source'd language file
    # extensions are the extensions this linter should run on
    extensions=()
    # files are the files this linter should run on
    files=()

    # Why: Dynamic
    # shellcheck disable=SC1090
    source "$DIR/linters/$languageName.sh"

    matched=false
    for extension in "${extensions[@]}"; do
      # If we don't find any files with the extension, skip the run.
      if [[ "$(git ls-files '*'."$extension" | wc -l | tr -d ' ')" -le 0 ]]; then
        continue
      fi
      matched=true
    done
    for file in "${files[@]}"; do
      # If there are no matching files, skip the run.
      if [[ "$(git ls-files "$file" | wc -l | tr -d ' ')" -le 0 ]]; then
        continue
      fi
      matched=true
    done
    if [[ $matched == "false" ]]; then
      exit 0
    fi

    # Note: extensions is set by the linter.
    # Why: We're OK with declaring and assigning.
    # shellcheck disable=SC2155,SC2001
    extensionsString=$(sed 's/ /,./g' <<<"${extensions[*]}" | sed 's/^/./')

    # show is used by run_command as metadata to be shown along with the command name
    show=$extensionsString

    # If we don't have any extensions, show the files this was ran on
    if [[ $extensionsString == "." ]]; then
      # shellcheck disable=SC2155,SC2001
      filesString=$(sed 's/ /,/g' <<<"${files[*]}")
      show=$filesString
    fi

    # Set by the language file
    if ! linter; then
      error "Linter failed to run, run 'make fmt' to fix"
      exit 1
    fi
  )
done
finished_at="$(get_time_ms)"
duration="$((finished_at - started_at))"
info "Linters took $(format_diff $duration)"
