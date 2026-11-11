#!/usr/bin/env bash
#
# Helper functions for BATS-based tests.
# Usage: add `load test_helper.sh` to your .bats file.

# mktempdir creates a temporary directory and echoes its path.
mktempdir() {
  local tmpdir="${TMPDIR:-/tmp}"
  local suffix="${1:-devbase-test-XXXXXX}"
  local dir="$tmpdir/$suffix"
  mktemp -d "$dir"
}

# setup_command_stubs creates the directories that stub_command writes
# to, and puts the stub directory first on PATH so that stubs shadow the
# real tools. Call it from `setup()` and teardown_command_stubs from
# `teardown()`.
setup_command_stubs() {
  STUB_DIR="$(mktempdir devbase-stubs-XXXXXX)"
  STUB_CALLS_FILE="$STUB_DIR/calls"
  STUB_OUTPUTS_DIR="$STUB_DIR/outputs"
  STUB_ARGS_DIR="$STUB_DIR/args"
  mkdir -p "$STUB_OUTPUTS_DIR" "$STUB_ARGS_DIR"
  : >"$STUB_CALLS_FILE"
  export STUB_DIR STUB_CALLS_FILE STUB_OUTPUTS_DIR STUB_ARGS_DIR
  export PATH="$STUB_DIR:$PATH"
}

# teardown_command_stubs removes the directory that setup_command_stubs
# created, and unsets the variables that point into it.
teardown_command_stubs() {
  rm -rf "$STUB_DIR"
  unset STUB_DIR STUB_CALLS_FILE STUB_OUTPUTS_DIR STUB_ARGS_DIR
}

# stub_command NAME OUTPUT [EXIT_CODES]
#
# Install an executable stub on PATH that records its invocation to
# $STUB_CALLS_FILE (one line: "NAME ARGS..."), records its full argv to
# $STUB_ARGS_DIR/NAME.argv (one arg per line, latest invocation only),
# and prints OUTPUT to stdout when the call exits 0 and to stderr otherwise.
#
# EXIT_CODES is a space-separated list with one exit code per call. The
# last code repeats for later calls, and the default is 0. For example,
# `stub_command gh v1 "1 1 0"` fails twice and then succeeds.
stub_command() {
  local name="$1" output="$2" exitCodes="${3:-0}"
  printf '%s' "$output" >"$STUB_OUTPUTS_DIR/$name"
  printf '%s' "$exitCodes" >"$STUB_OUTPUTS_DIR/$name.exit"
  cat >"$STUB_DIR/$name" <<'EOF'
#!/usr/bin/env bash
name="$(basename "$0")"
echo "$name $*" >>"$STUB_CALLS_FILE"
printf '%s\n' "$@" >"$STUB_ARGS_DIR/$name.argv"
read -r -a codes <"$STUB_OUTPUTS_DIR/$name.exit"
index="$(($(grep -c "^$name " "$STUB_CALLS_FILE") - 1))"
if ((index >= ${#codes[@]})); then
  index=$((${#codes[@]} - 1))
fi
exitCode="${codes[$index]}"
if ((exitCode == 0)); then
  cat "$STUB_OUTPUTS_DIR/$name"
else
  cat "$STUB_OUTPUTS_DIR/$name" >&2
fi
exit "$exitCode"
EOF
  chmod +x "$STUB_DIR/$name"
}

# stub_function NAME [EXIT_CODE]
#
# Define a shell function NAME that records its invocation to
# $STUB_CALLS_FILE in the same "NAME ARGS..." format as stub_command, then
# returns EXIT_CODE (default 0). Use it for functions that the code under
# test calls, because stub_command only replaces executables. The calls go
# to a file so that they remain visible after `run`, which executes the
# function in a subshell.
stub_function() {
  local name="$1" exitCode="${2:-0}"
  eval "$name() {
    echo \"$name \$*\" >>\"\$STUB_CALLS_FILE\"
    return $exitCode
  }"
}

# stub_argv NAME prints the arguments of the latest invocation of the
# named stub, one per line.
stub_argv() {
  cat "$STUB_ARGS_DIR/$1.argv"
}

# assert_stub_called NAME ARGS... asserts that the named stub ran at
# least once with these arguments.
assert_stub_called() {
  # -F treats the call as literal text, so regex metacharacters match
  # themselves. -x requires the whole line to match, so a call with extra
  # arguments does not. -q hides grep's output, since the failure branch
  # prints its own message. The call log joins arguments with spaces, so
  # one argument that contains a space is the same as two arguments.
  if ! grep -Fxq -- "$*" "$STUB_CALLS_FILE"; then
    {
      echo "expected stub call: $*"
      echo "recorded calls:"
      cat "$STUB_CALLS_FILE"
    } >&2
    return 1
  fi
}

# assert_stub_call_count NAME COUNT asserts that the named stub ran
# exactly COUNT times.
assert_stub_call_count() {
  local name="$1" count="$2"
  # `run` swallows grep's non-zero exit when the count is 0, so we can
  # assert the count directly without an explicit failure-path check.
  run grep -c "^$name " "$STUB_CALLS_FILE"
  assert_output "$count"
}

# assert_stub_not_called NAME asserts that the named stub never ran.
assert_stub_not_called() {
  assert_stub_call_count "$1" 0
}
