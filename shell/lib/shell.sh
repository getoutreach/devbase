#!/usr/bin/env bash
# Provides generic shell helper functions

DEVBASE_CACHED_BINARY_STORAGE_PATH="$HOME/.outreach/.cache/devbase"

# retry calls a given command (must be wrapped in quotes)
# syntax: retry <interval> <maxRetries> <command> [args...]
retry() {
  local interval="$1"
  local maxRetries="$2"
  local command="$3"

  # remove interval+command from the argument stack
  # so we can send it to the command later
  shift
  shift
  shift

  local exitCode=0
  for i in $(seq 1 "$maxRetries"); do
    if [[ $i -gt 1 ]]; then
      echo "RETRYING: $command ($i/$maxRetries)"
    fi

    # execute command, if succeeds break out of loop and
    # and reset exitCode
    "$command" "$@" && exitCode=0 && break

    # preserve the exit code
    exitCode=$?

    # try again after x time
    sleep "$interval"
  done

  return $exitCode
}

# cached_binary_path returns the raw path of a binary if it
# were to be cached. It is not guaranteed to exist.
cached_binary_path() {
  local name="$1"
  local version="$2"

  echo "$DEVBASE_CACHED_BINARY_STORAGE_PATH/$name/$version/$name"
}

# get_cached_binary returns the path to a cached binary
# or returns empty if not found
get_cached_binary() {
  local name="$1"
  local version="$2"

  # shellcheck disable=SC2155 # Why: No return value
  local cachedPath=$(cached_binary_path "$name" "$version")

  # Create the base path.
  mkdir -p "$(dirname "$cachedPath")"

  if [[ ! -e $cachedPath ]]; then
    echo ""
  else
    echo "$cachedPath"
  fi
}

# get_time_ms returns the current time in milliseconds
# we use perl because we can't use the date command %N on macOS
get_time_ms() {
  perl -MTime::HiRes -e 'printf("%.0f\n",Time::HiRes::time()*1000)'
}

# is_terminal returns true if the current process is a terminal, with
# a special case of always being false if CI is true
is_terminal() {
  [[ -t 0 ]] && [[ $CI != "true" ]]
}

# get_cursor_pos returns the current cursor position
get_cursor_pos() {
  # If not a tty, return 0,0
  if ! is_terminal; then
    echo "0,0"
    return
  fi

  # based on a script from http://invisible-island.net/xterm/xterm.faq.html
  exec </dev/tty
  oldstty=$(stty -g)
  stty raw -echo min 0
  # on my system, the following line can be replaced by the line below it
  echo -en "\033[6n" >/dev/tty
  # tput u7 > /dev/tty    # when TERM=xterm (and relatives)
  IFS=';' read -r -d R -a pos
  stty "$oldstty"
  # change from one-based to zero based so they work with: tput cup $row $col
  row=$((${pos[0]:2} - 1)) # strip off the esc-[
  col=$((pos[1] - 1))
  echo "$row,$col"
}

# run_command is a helper for running a command and timing it, showing
# the status
run_command() {
  local name="$1"
  local cmd="$2"
  shift
  shift
  local args=("$@")

  # show is metadata to be shown along with the command name
  local show=$show

  # Why: We're OK with declaring and assigning.
  # shellcheck disable=SC2155
  local started_at="$(get_time_ms)"
  info_sub "$name ($show)"

  current_pos="$(get_cursor_pos)"
  "$cmd" "${args[@]}"
  after_pos="$(get_cursor_pos)"
  exit_code=$?

  # Get how long it took to run the linter
  # Why: We're OK with declaring and assigning.
  # shellcheck disable=SC2155
  local finished_at="$(get_time_ms)"
  local duration="$((finished_at - started_at))"

  if is_terminal; then
    # If the position of the cursor didn't change, we can safely assume
    # that the linter didn't output anything, so we can just overwrite
    # the previous line.
    if [[ $current_pos == "$after_pos" ]]; then
      tput cuu1 || true
    fi
  fi

  # Rewrite the above line, or append, with the time taken
  info_sub "$name ($show) ($(format_diff $duration))"

  # If we failed, display an error and exit
  if [[ $exit_code -ne 0 ]]; then
    error "$name failed with exit code $exit_code"
    exit 1
  fi
}

# format_diff takes a diff and formats it into a friendly timestamp
format_diff() {
  local diff="$1"
  local seconds=$((diff / 1000))
  local ms=$((diff % 1000))
  printf "%d.%02ds" "$seconds" "$ms"
}
