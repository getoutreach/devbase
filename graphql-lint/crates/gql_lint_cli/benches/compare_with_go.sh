#!/usr/bin/env bash
# Process-level timing comparison between this Rust port and the shipped
# Go `devbase graphql lint` binary, over the same real corpus
# (getoutreach/giraffe, branch claude/devbase-graphql-lint-config).
#
# This is deliberately NOT a criterion benchmark: criterion measures
# in-process Rust code (see benches/lint.rs), and the Go binary is a
# separate process written in a separate language — there's no fair way
# for criterion itself to time it. This script instead times both real
# binaries as black boxes, the same way a user would actually invoke
# either one, and reports wall-clock time averaged over several runs.
#
# Usage:
#   crates/gql_lint_cli/benches/compare_with_go.sh <rust-binary> <go-binary> [giraffe-dir] [iterations]
#
# Example (from the graphql-lint workspace root, after building both):
#   cargo build --release
#   crates/gql_lint_cli/benches/compare_with_go.sh \
#     target/release/gql-lint /tmp/devbase-go /home/user/giraffe 5

set -euo pipefail

RUST_BIN_ARG="${1:?usage: compare_with_go.sh <rust-binary> <go-binary> [giraffe-dir] [iterations]}"
GO_BIN_ARG="${2:?usage: compare_with_go.sh <rust-binary> <go-binary> [giraffe-dir] [iterations]}"
GIRAFFE_DIR="${3:-/home/user/giraffe}"
ITERATIONS="${4:-5}"

if [[ ! -x "$RUST_BIN_ARG" ]]; then
  echo "error: $RUST_BIN_ARG is not an executable file" >&2
  exit 1
fi
if [[ ! -x "$GO_BIN_ARG" ]]; then
  echo "error: $GO_BIN_ARG is not an executable file" >&2
  exit 1
fi

# Both binaries get invoked from inside $GIRAFFE_DIR below (to match how a
# user would really run either one), so a relative path given on the
# command line must be resolved to absolute now, before any `cd`.
RUST_BIN="$(realpath "$RUST_BIN_ARG")"
GO_BIN="$(realpath "$GO_BIN_ARG")"
if [[ ! -d "$GIRAFFE_DIR/.git" ]]; then
  echo "error: $GIRAFFE_DIR does not look like a git checkout of getoutreach/giraffe" >&2
  exit 1
fi

# src/modules/{extensibility,support-admin}/schema.graphql have a real,
# pre-existing bug unrelated to either tool: `@markeplaceAuth(actors:
# [user, admin])` passes bare, unquoted enum-shaped literals where the
# directive itself declares `actors: [String!]!`. gqlparser (the Go
# tool's parser) accepts this silently; apollo-compiler correctly rejects
# it as a spec violation -- a genuine, confirmed strictness difference
# between the two libraries, not a bug in this port (see benches/lint.rs'
# own EXCLUDED_FOR_BENCHMARK and the plan file's tenth session write-up
# for the full story). Left in, the Rust binary fails at Tier 1 for the
# whole corpus and this script would only be comparing "Go succeeds" to
# "Rust reports one syntax-ish error," which is neither a correctness nor
# a timing comparison worth trusting. Excluded here for the same
# benchmarking-only reason -- never patch giraffe's own files to work
# around this.
EXCLUDES=(-e "src/modules/extensibility/**" -e "src/modules/support-admin/**")

# Correctness sanity check first: a timing comparison is worthless if the
# two tools don't agree on what they're linting. Not exact-parity (this
# port has documented, known message-text/position differences from a
# few rules -- see the plan file's tenth session write-up) but the
# per-rule violation *counts* should be close; a large unexplained gap
# means don't trust the numbers below until it's understood.
echo "== Correctness sanity check (rule -> violation count, excluding the known-bug files above) =="
rust_counts="$(cd "$GIRAFFE_DIR" && "$RUST_BIN" "${EXCLUDES[@]}" . 2>/dev/null | grep -o '\[[a-z-]*\]' | sort | uniq -c)" || true
go_counts="$(cd "$GIRAFFE_DIR" && "$GO_BIN" lint graphql "${EXCLUDES[@]}" . 2>/dev/null | grep -o '\[[a-z-]*\]' | sort | uniq -c)" || true
echo "-- rust --"
echo "$rust_counts"
echo "-- go --"
echo "$go_counts"
echo

# Timing: both tools invoked exactly as a user would, from inside the
# repo being linted, over $ITERATIONS runs each. `time` (not `/usr/bin/time
# -v`) keeps this portable; redirect stdout since we only want the timing,
# and each tool's own stderr config-resolution line is harmless noise.
time_runs() {
  local label="$1"
  shift
  local total=0
  for ((i = 1; i <= ITERATIONS; i++)); do
    local start end elapsed
    start=$(date +%s.%N)
    (cd "$GIRAFFE_DIR" && "$@" >/dev/null 2>&1) || true
    end=$(date +%s.%N)
    elapsed=$(echo "$end - $start" | bc)
    echo "  run $i: ${elapsed}s"
    total=$(echo "$total + $elapsed" | bc)
  done
  local avg
  avg=$(echo "$total / $ITERATIONS" | bc -l)
  printf '%s average over %d runs: %.3fs\n\n' "$label" "$ITERATIONS" "$avg"
}

echo "== Timing ($ITERATIONS runs each) =="
time_runs "gql-lint (Rust, sequential)" "$RUST_BIN" "${EXCLUDES[@]}" .
time_runs "gql-lint (Rust, --parallel)" "$RUST_BIN" --parallel "${EXCLUDES[@]}" .
time_runs "devbase graphql lint (Go)" "$GO_BIN" lint graphql "${EXCLUDES[@]}" .
