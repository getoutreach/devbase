// Copyright 2026 Outreach Corporation. Licensed under the Apache License 2.0.

// Description: End-to-end tests for the "yq" CLI subcommand.

package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/urfave/cli/v3"
	"gotest.tools/v3/assert"
)

// runYqCommand runs the "yq" subcommand with args (not including the
// command name itself) against stdin, and returns stdout, stderr, and
// any error cli/v3 returns from Run (an *cli.exitCoder wrapping a
// non-zero exit code on failure).
func runYqCommand(t *testing.T, stdin string, args ...string) (stdout, stderr string, err error) {
	t.Helper()

	cmd := newYqCommand()
	var out, errOut bytes.Buffer
	cmd.Reader = strings.NewReader(stdin)
	cmd.Writer = &out
	cmd.ErrWriter = &errOut
	cmd.ExitErrHandler = func(context.Context, *cli.Command, error) {}

	runErr := cmd.Run(context.Background(), append([]string{"yq"}, args...))
	return out.String(), errOut.String(), runErr
}

// exitCode extracts the exit code cli.Exit encoded into err, failing
// the test if err doesn't carry one.
func exitCode(t *testing.T, err error) int {
	t.Helper()
	var exitErr cli.ExitCoder
	ok := errors.As(err, &exitErr)
	assert.Assert(t, ok, err)
	return exitErr.ExitCode()
}

// TestYqCommand_StdinFilter confirms `devbase yq <filter>` reads from
// stdin and writes formatted JSON to stdout, matching how
// shell/yq.sh's real callers redirect a file into it.
func TestYqCommand_StdinFilter(t *testing.T) {
	stdout, _, err := runYqCommand(t, "arguments:\n  enableCgo: true\n", "-r", ".arguments.enableCgo")
	assert.NilError(t, err)
	assert.Equal(t, stdout, "true\n")
}

// TestYqCommand_YAMLOutput confirms -y produces YAML output end to
// end through the CLI command.
func TestYqCommand_YAMLOutput(t *testing.T) {
	stdout, _, err := runYqCommand(t, "b: 2\na: 1\n", "-y", ".")
	assert.NilError(t, err)
	assert.Equal(t, stdout, "b: 2\na: 1\n")
}

// TestYqCommand_FileArgument confirms a file operand (not just stdin)
// is read directly, matching call sites that pass a filename rather
// than redirecting stdin.
func TestYqCommand_FileArgument(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	assert.NilError(t, os.WriteFile(path, []byte("name: myservice\n"), 0o600))

	stdout, _, err := runYqCommand(t, "", "-r", ".name", path)
	assert.NilError(t, err)
	assert.Equal(t, stdout, "myservice\n")
}

// TestYqCommand_InPlace confirms -i edits the given file through the
// full CLI command.
func TestYqCommand_InPlace(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	assert.NilError(t, os.WriteFile(path, []byte("count: 1\n"), 0o600))

	_, _, err := runYqCommand(t, "", "-i", ".count = 2", path)
	assert.NilError(t, err)

	got, err := os.ReadFile(path)
	assert.NilError(t, err)
	assert.Equal(t, string(got), "count: 2\n")
}

// TestYqCommand_InPlaceRejectsStdin confirms -i without a file operand
// is a distinct, actionable error rather than silently reading stdin.
func TestYqCommand_InPlaceRejectsStdin(t *testing.T) {
	_, stderr, err := runYqCommand(t, "count: 1\n", "-i", ".count = 2")
	assert.Assert(t, err != nil)
	assert.Equal(t, exitCode(t, err), 1)
	assert.Assert(t, strings.Contains(stderr, "requires at least one file"), stderr)
}

// TestYqCommand_DeferredFlag confirms a recognized-but-deferred flag
// produces its distinct message and a non-zero exit code, rather than
// the generic "unrecognized flag" error.
func TestYqCommand_DeferredFlag(t *testing.T) {
	_, stderr, err := runYqCommand(t, "a: 1\n", "-f", ".")
	assert.Assert(t, err != nil)
	assert.Equal(t, exitCode(t, err), 1)
	assert.Assert(t, strings.Contains(stderr, "does not implement yet"), stderr)
}

// TestYqCommand_UnknownFlag confirms a genuinely unrecognized flag
// exits non-zero with its own distinct error.
func TestYqCommand_UnknownFlag(t *testing.T) {
	_, stderr, err := runYqCommand(t, "a: 1\n", "--not-a-real-flag", ".")
	assert.Assert(t, err != nil)
	assert.Equal(t, exitCode(t, err), 1)
	assert.Assert(t, strings.Contains(stderr, "unknown flag"), stderr)
}

// TestYqCommand_SlurpInPlace confirms -s combined with -i slurps the
// file's own documents into one array before running the filter,
// instead of silently ignoring -s and running per-document (the -si
// combination's real bug, found and fixed during review).
func TestYqCommand_SlurpInPlace(t *testing.T) {
	path := filepath.Join(t.TempDir(), "multi.yaml")
	assert.NilError(t, os.WriteFile(path, []byte("a: 1\n---\na: 2\n"), 0o600))

	_, _, err := runYqCommand(t, "", "-si", ". | length", path)
	assert.NilError(t, err)

	got, err := os.ReadFile(path)
	assert.NilError(t, err)
	assert.Equal(t, string(got), "2\n")
}

// TestYqCommand_MultipleFileOperands confirms multiple file operands
// are read as separate YAML documents (joined by "---", not a bare
// newline), so two files each with a top-level "name:" key don't
// collide as a duplicate-key error (the real bug readInput had,
// found and fixed during review).
func TestYqCommand_MultipleFileOperands(t *testing.T) {
	dir := t.TempDir()
	pathA := filepath.Join(dir, "a.yaml")
	pathB := filepath.Join(dir, "b.yaml")
	assert.NilError(t, os.WriteFile(pathA, []byte("name: a\n"), 0o600))
	assert.NilError(t, os.WriteFile(pathB, []byte("name: b\n"), 0o600))

	stdout, _, err := runYqCommand(t, "", "-r", ".name", pathA, pathB)
	assert.NilError(t, err)
	assert.Equal(t, stdout, "a\nb\n")
}

// TestYqCommand_YAMLRoundtripHonorsSortAndIndentless confirms -Y
// combined with -S/--indentless-lists actually applies them, instead
// of Roundtrip silently ignoring FormatOptions (the real bug found and
// fixed during review).
func TestYqCommand_YAMLRoundtripHonorsSortAndIndentless(t *testing.T) {
	stdout, _, err := runYqCommand(t, "zeta: 1\nalpha: 2\n", "-Y", "-S", ".")
	assert.NilError(t, err)
	assert.Equal(t, stdout, "alpha: 2\nzeta: 1\n")

	stdout, _, err = runYqCommand(t, "list:\n  - a\n  - b\n", "-Y", "--indentless-lists", ".")
	assert.NilError(t, err)
	assert.Equal(t, stdout, "list:\n- a\n- b\n")
}

// TestYqCommand_ErrorIncludesFilterText confirms a jq-level runtime
// error names the actual filter that failed, not a placeholder (the
// real gap found and fixed during review: Engine didn't retain the
// filter string it was compiled from).
func TestYqCommand_ErrorIncludesFilterText(t *testing.T) {
	_, stderr, err := runYqCommand(t, "a: 1\n", `.a | error("boom")`)
	assert.Assert(t, err != nil)
	assert.Assert(t, strings.Contains(stderr, `.a | error("boom")`), stderr)
}

// TestYqCommand_Version confirms -V/--version works standalone,
// without a filter argument, and identifies devbase yq rather than
// falling through to "missing filter argument" (the real gap found
// and fixed during review: the flag was parsed and validated as
// implemented but never actually checked).
func TestYqCommand_Version(t *testing.T) {
	stdout, _, err := runYqCommand(t, "", "--version")
	assert.NilError(t, err)
	assert.Assert(t, strings.Contains(stdout, "devbase yq"), stdout)

	stdout, _, err = runYqCommand(t, "", "-V")
	assert.NilError(t, err)
	assert.Assert(t, strings.Contains(stdout, "devbase yq"), stdout)
}

// TestYqCommand_PartialOutputBeforeError confirms a filter that emits
// a value and then errors still prints the earlier value, matching
// real jq/gojq CLI behavior (`jq -c '1, error("boom")'` prints 1
// before the error) -- the real bug found and fixed during review,
// where renderDocument discarded every value already rendered before
// a later error.
func TestYqCommand_PartialOutputBeforeError(t *testing.T) {
	stdout, stderr, err := runYqCommand(t, "a: 1\n", `1, error("boom")`)
	assert.Assert(t, err != nil)
	assert.Equal(t, stdout, "1\n")
	assert.Assert(t, strings.Contains(stderr, "boom"), stderr)
}

// TestYqCommand_YAMLRoundtrip confirms -Y preserves comments on
// untouched keys end to end through the CLI command, implying YAML
// output even without an explicit -y.
func TestYqCommand_YAMLRoundtrip(t *testing.T) {
	stdout, _, err := runYqCommand(t, "kind: ConfigMap # keep\ndata: old # dropped\n",
		"-Y", ". * {data: {new: 1}}")
	assert.NilError(t, err)
	assert.Equal(t, stdout, "kind: ConfigMap # keep\ndata:\n  new: 1\n")
}

// TestYqCommand_BoxShMerge exercises box.sh:52-53's real filter end to
// end through the CLI command: --slurpfile plus a merge. The merge
// (`*`) allocates a brand-new result map, the same way a mutating
// filter does (see format_test.go's TestFormatValue_KeyOrder_
// MutationSplit), so the root's recorded key order does not survive
// and it falls back to alphabetical ("config" before "name") -- an
// accepted limitation of the default (non -Y) key-order design, not a
// bug.
func TestYqCommand_BoxShMerge(t *testing.T) {
	boxPath := filepath.Join(t.TempDir(), "box.yaml")
	assert.NilError(t, os.WriteFile(boxPath, []byte("org: outreach\n"), 0o600))

	stdout, _, err := runYqCommand(t, "name: myservice\n",
		"--yaml-output", "--slurpfile", "boxconf", boxPath, ". * {config: $boxconf[0]}")
	assert.NilError(t, err)
	assert.Equal(t, stdout, "config:\n  org: outreach\nname: myservice\n")
}
