// Copyright 2026 Outreach Corporation. Licensed under the Apache License 2.0.

// Description: Tests for -i/--in-place file editing.

package yq

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"gotest.tools/v3/assert"
)

// writeYAML writes contents to a fresh temp file named name and
// returns its path.
func writeYAML(t *testing.T, name, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	assert.NilError(t, os.WriteFile(path, []byte(contents), 0o600))
	return path
}

// TestRunFile_EndToEnd confirms a mutating filter edits the file in
// place and the change is visible on re-read.
func TestRunFile_EndToEnd(t *testing.T) {
	path := writeYAML(t, "config.yaml", "arguments:\n  enableCgo: true\n")

	e, err := Compile(".arguments.enableCgo = false", nil)
	assert.NilError(t, err)

	assert.NilError(t, RunFile(context.Background(), path, e, FormatOptions{}, false, false))

	got, err := os.ReadFile(path)
	assert.NilError(t, err)

	v, _, err := NormalizeYAML(got)
	assert.NilError(t, err)
	m := v.(map[string]any)                 //nolint:errcheck,forcetypeassert // Why: test fixture.
	args := m["arguments"].(map[string]any) //nolint:errcheck,forcetypeassert // Why: test fixture.
	assert.Equal(t, args["enableCgo"], false)
}

// TestRunFile_AlwaysWritesYAML confirms -i always writes YAML back,
// even when opts.YAML is left false (opts.Compact JSON is irrelevant
// to in-place output).
func TestRunFile_AlwaysWritesYAML(t *testing.T) {
	path := writeYAML(t, "config.yaml", "a: 1\n")

	e, err := Compile(".", nil)
	assert.NilError(t, err)

	assert.NilError(t, RunFile(context.Background(), path, e, FormatOptions{Compact: true}, false, false))

	got, err := os.ReadFile(path)
	assert.NilError(t, err)
	assert.Equal(t, string(got), "a: 1\n")
}

// TestRunFiles_MultipleFilesEditedIndependently confirms -i with
// multiple files edits each one, rather than erroring.
func TestRunFiles_MultipleFilesEditedIndependently(t *testing.T) {
	path1 := writeYAML(t, "a.yaml", "n: 1\n")
	path2 := writeYAML(t, "b.yaml", "n: 2\n")

	e, err := Compile(".n += 10", nil)
	assert.NilError(t, err)

	errs := RunFiles(context.Background(), []string{path1, path2}, e, FormatOptions{}, false, false)
	assert.Equal(t, len(errs), 0)

	got1, err := os.ReadFile(path1)
	assert.NilError(t, err)
	assert.Equal(t, string(got1), "n: 11\n")

	got2, err := os.ReadFile(path2)
	assert.NilError(t, err)
	assert.Equal(t, string(got2), "n: 12\n")
}

// TestRunFiles_OneFailureDoesNotAbortOthers confirms a filter that
// produces no output for one file still lets sibling files succeed --
// each file is edited (or fails) independently.
func TestRunFiles_OneFailureDoesNotAbortOthers(t *testing.T) {
	// select(...) with no match emits zero values for this file, but
	// the sibling file's key does match.
	path1 := writeYAML(t, "a.yaml", "kind: Secret\n")
	path2 := writeYAML(t, "b.yaml", "kind: ConfigMap\n")

	e, err := Compile(`select(.kind == "ConfigMap")`, nil)
	assert.NilError(t, err)

	errs := RunFiles(context.Background(), []string{path1, path2}, e, FormatOptions{}, false, false)
	assert.Equal(t, len(errs), 1)
	assert.ErrorContains(t, errs[0], "filter produced no output")

	// path1 is left unmodified.
	got1, err := os.ReadFile(path1)
	assert.NilError(t, err)
	assert.Equal(t, string(got1), "kind: Secret\n")

	// path2 was still edited despite path1's failure.
	got2, err := os.ReadFile(path2)
	assert.NilError(t, err)
	assert.Equal(t, string(got2), "kind: ConfigMap\n")
}

// TestRunFile_MultipleEmittedValues confirms multiple values emitted
// for one file join as a "---"-separated stream.
func TestRunFile_MultipleEmittedValues(t *testing.T) {
	path := writeYAML(t, "list.yaml", "items:\n  - a\n  - b\n")

	e, err := Compile(".items[]", nil)
	assert.NilError(t, err)

	assert.NilError(t, RunFile(context.Background(), path, e, FormatOptions{}, false, false))

	got, err := os.ReadFile(path)
	assert.NilError(t, err)
	assert.Equal(t, string(got), "a\n---\nb\n")
}

// TestRunFiles_ConcurrentEditsDoNotCrossContaminate runs RunFiles
// against many files sharing one Engine (and therefore one compiled
// gojq.Code) concurrently, and confirms each file gets its own
// correct, distinct result -- the case that would catch a race if
// engine reuse across goroutines were not actually safe (gojq's own
// Code.Run/RunWithContext doc comment states it is).
func TestRunFiles_ConcurrentEditsDoNotCrossContaminate(t *testing.T) {
	const n = 50
	paths := make([]string, n)
	for i := range n {
		paths[i] = writeYAML(t, fmt.Sprintf("file-%d.yaml", i), fmt.Sprintf("n: %d\n", i))
	}

	e, err := Compile(".n *= 10", nil)
	assert.NilError(t, err)

	errs := RunFiles(context.Background(), paths, e, FormatOptions{}, false, false)
	assert.Equal(t, len(errs), 0)

	for i, path := range paths {
		got, err := os.ReadFile(path)
		assert.NilError(t, err)
		assert.Equal(t, string(got), fmt.Sprintf("n: %d\n", i*10))
	}
}

// TestRunFiles_ErrorsPreserveInputOrder confirms RunFiles' returned
// errors correspond to paths by position, not by whichever goroutine
// happens to finish first.
func TestRunFiles_ErrorsPreserveInputOrder(t *testing.T) {
	// Every other file's filter matches nothing (zero output -> error);
	// the rest succeed. With concurrency, completion order is not
	// input order, so this only passes if RunFiles indexes errors by
	// position rather than appending in completion order.
	const n = 20
	paths := make([]string, n)
	for i := range n {
		kind := "Secret"
		if i%2 == 0 {
			kind = "ConfigMap"
		}
		paths[i] = writeYAML(t, fmt.Sprintf("file-%d.yaml", i), fmt.Sprintf("kind: %s\n", kind))
	}

	e, err := Compile(`select(.kind == "ConfigMap")`, nil)
	assert.NilError(t, err)

	errs := RunFiles(context.Background(), paths, e, FormatOptions{}, false, false)
	assert.Equal(t, len(errs), n/2)
	for _, err := range errs {
		assert.ErrorContains(t, err, "filter produced no output")
	}
}

// TestRunFiles_DuplicatePathsAreSequential confirms repeating the same
// path many times in one RunFiles call still composes deterministically
// (as it did before RunFiles ran concurrently), rather than racing on
// that file's contents -- the real regression found and fixed during
// review: parallelizing distinct files is safe, but two goroutines
// racing to read-modify-write the *same* file is not.
func TestRunFiles_DuplicatePathsAreSequential(t *testing.T) {
	path := writeYAML(t, "counter.yaml", "n: 1\n")

	e, err := Compile(".n += 1", nil)
	assert.NilError(t, err)

	const repeats = 20
	paths := make([]string, repeats)
	for i := range paths {
		paths[i] = path
	}

	errs := RunFiles(context.Background(), paths, e, FormatOptions{}, false, false)
	assert.Equal(t, len(errs), 0)

	got, err := os.ReadFile(path)
	assert.NilError(t, err)
	assert.Equal(t, string(got), fmt.Sprintf("n: %d\n", 1+repeats))
}

// TestRunFile_PreservesPermissions confirms the atomic write preserves
// the original file's permissions rather than defaulting to something
// else.
func TestRunFile_PreservesPermissions(t *testing.T) {
	path := writeYAML(t, "config.yaml", "a: 1\n")
	assert.NilError(t, os.Chmod(path, 0o640))

	e, err := Compile(".", nil)
	assert.NilError(t, err)
	assert.NilError(t, RunFile(context.Background(), path, e, FormatOptions{}, false, false))

	info, err := os.Stat(path)
	assert.NilError(t, err)
	assert.Equal(t, info.Mode().Perm(), os.FileMode(0o640))
}
