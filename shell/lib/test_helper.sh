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

# stub_command NAME OUTPUT [EXIT_CODE]
#
# Install an executable stub on PATH that records its invocation to
# $STUB_CALLS_FILE (one line: "NAME ARGS..."), records its full argv to
# $STUB_ARGS_DIR/NAME.argv (one arg per line, latest invocation only),
# prints OUTPUT to stdout, and exits with EXIT_CODE (default 0).
stub_command() {
  local name="$1" output="$2" exitCode="${3:-0}"
  printf '%s' "$output" >"$STUB_OUTPUTS_DIR/$name"
  printf '%s' "$exitCode" >"$STUB_OUTPUTS_DIR/$name.exit"
  cat >"$STUB_DIR/$name" <<'EOF'
#!/usr/bin/env bash
name="$(basename "$0")"
echo "$name $*" >>"$STUB_CALLS_FILE"
printf '%s\n' "$@" >"$STUB_ARGS_DIR/$name.argv"
cat "$STUB_OUTPUTS_DIR/$name"
exit "$(cat "$STUB_OUTPUTS_DIR/$name.exit")"
EOF
  chmod +x "$STUB_DIR/$name"
}

# stub_argv NAME prints the arguments of the latest invocation of the
# named stub, one per line.
stub_argv() {
  cat "$STUB_ARGS_DIR/$1.argv"
}

# assert_stub_not_called NAME asserts that the named stub never ran.
assert_stub_not_called() {
  local name="$1"
  # `run` swallows grep's non-zero exit when the count is 0, so we can
  # assert the count directly without an explicit failure-path check.
  run grep -c "^$name " "$STUB_CALLS_FILE"
  assert_output "0"
}
