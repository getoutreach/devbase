#!/usr/bin/env bash
set -e -o pipefail

# The linter is flaky in some environments so we allow it to be overridden.
# Also, if your editor already supports linting, you can make your tests run
# faster at little cost with:
# `LINTER=/bin/true make test``
DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"

# shellcheck source=./lib/logging.sh
source "$DIR/lib/logging.sh"
# shellcheck source=./lib/bootstrap.sh
source "$DIR/lib/bootstrap.sh"
# shellcheck source=./languages/nodejs.sh
source "$DIR/languages/nodejs.sh"

# get_time_ms returns the current time in milliseconds
# we use perl because we can't use the date command %N on macOS
get_time_ms() {
  perl -MTime::HiRes -e 'printf("%.0f\n",Time::HiRes::time()*1000)'
}

run_linter() {
  local linter_name="$1"
  shift
  local linter_bin="$1"
  shift
  local linter_args=("$@")

  # Note: extensions is set by the linter.
  # Why: We're OK with declaring and assigning.
  # shellcheck disable=SC2155,SC2001
  local extensions=$(sed 's/ /,./' <<<"${extensions[*]}" | sed 's/^/./')
  # shellcheck disable=SC2155,SC2001
  local files=$(sed 's/ /,/' <<<"${files[*]}")
  local show=$extensions
  if [[ $extensions == "." ]]; then
    show=$files
  fi

  # Why: We're OK with declaring and assigning.
  # shellcheck disable=SC2155
  local started_at="$(get_time_ms)"
  info_sub "$linter_name ($show)"
  "$linter_bin" "${linter_args[@]}"
  exit_code=$?
  # Why: We're OK with declaring and assigning.
  # shellcheck disable=SC2155
  local finished_at="$(get_time_ms)"
  local duration="$((finished_at - started_at))"
  if [[ $exit_code -ne 0 ]]; then
    error "$linter_name failed with exit code $exit_code"
    exit 1
  fi
  # Move the cursor back up, but ignore failure when we don't have a terminal
  tput cuu1 || true
  info_sub "$linter_name ($show) ($(format_diff $duration))"
}

format_diff() {
  local diff="$1"
  local seconds=$((diff / 1000))
  local ms=$((diff % 1000))
  printf "%d.%02ds" "$seconds" "$ms"
}

if [[ -n $SKIP_LINTERS ]] || [[ -n $SKIP_VALIDATE ]]; then
  info "Skipping linters"
  exit 0
fi

info "Running linters"

started_at="$(get_time_ms)"
for language in "$DIR/linters"/*.sh; do
  language="$(basename "$language")"
  language="${language%.sh}"

  # We use a sub-shell to prevent inheriting
  # the changes to functions/variables to the parent
  # (this) script
  (
    # Modified by the language file
    extensions=()
    files=()

    # Why: Dynamic
    # shellcheck disable=SC1090
    source "$DIR/linters/$language.sh"

    matched=false
    for extension in "${extensions[@]}"; do
      # If we don't find any files with the extension, skip the run.
      if [[ "$(git ls-files "*.$extension" | wc -l | tr -d ' ')" -le 0 ]]; then
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

    # Set by the language file
    linter
  )
done
finished_at="$(get_time_ms)"
duration="$((finished_at - started_at))"
info "Linters took $(format_diff $duration)"
