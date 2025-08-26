#!/usr/bin/env bash

set -euo pipefail

if [[ $# -lt 2 ]]; then
  echo "Usage: $0 <make_task> <mise_task> [args...]" >&2
  exit 1
fi

make_task="$1"
mise_task="$2"
shift
shift

deprecated_msg() {
  echo >&2
  # Bold, blinking, red text
  echo -e "\e[1m\e[5m\e[31m**DEPRECATION MESSAGE**\e[0m" >&2
  echo "'make $make_task' is deprecated. Use 'mise run $mise_task' instead" >&2
  echo >&2
}

deprecated_msg

trap deprecated_msg EXIT

echo "Starting in 5 seconds..." >&2
echo >&2
sleep 5

mise run --quiet "$mise_task" "$@"
