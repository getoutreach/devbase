#!/usr/bin/env bash
# Emulator for the bootstrap scripts/shell-wrapper.sh to use ourself.

command="$1"
shift
exec "shell/$command" "$@"
