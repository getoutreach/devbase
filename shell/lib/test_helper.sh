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
