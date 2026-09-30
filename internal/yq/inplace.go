// Copyright 2026 Outreach Corporation. Licensed under the Apache License 2.0.

// Description: -i/--in-place file editing.

package yq

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	yaml "github.com/itchyny/go-yaml"
	"golang.org/x/sync/errgroup"
)

// RunFile evaluates the compiled filter against the YAML document(s)
// in path and overwrites path with the result, always as YAML
// regardless of opts.YAML (python-yq itself requires an explicit
// non-JSON output flag alongside -i; devbase yq sidesteps that by
// always writing YAML for in-place edits). Under slurp, path's
// documents are combined into one array and the filter runs once
// against it, the same way runToStdout's non -i slurp does; otherwise
// each document is run through the filter independently, in its
// original order. Every emitted value joins into one "---"-separated
// stream. Zero emitted values is an error, and leaves path unmodified.
// Under roundtrip (-Y, and not slurp -- see runToStdout's own doc
// comment on why slurp and -Y don't combine), each document's result
// is rendered via Roundtrip against that document's own original
// parse tree instead of via plain FormatValue.
//
// The write is temp-file-plus-rename, never truncate-then-write, so an
// interrupted write leaves the original file intact instead of a
// broken, empty one.
func RunFile(ctx context.Context, path string, engine *Engine, opts FormatOptions, slurp, roundtrip bool) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}

	docs, nodes, order, err := SplitYAMLDocumentsWithNodes(b)
	if err != nil {
		return fmt.Errorf("decode %s: %w", path, err)
	}
	if slurp {
		docs, nodes, roundtrip = []any{docs}, []*yaml.Node{nil}, false
	}

	opts.YAML = true
	opts.Order = order

	var rendered []string
	for i, doc := range docs {
		// Unlike runToStdout, a partial result here is deliberately
		// discarded rather than written: an incomplete in-place edit
		// on disk is worse than none, whereas printing partial output
		// before an error is normal, expected jq/gojq CLI behavior.
		out, err := renderDocument(ctx, doc, nodes[i], engine, opts, roundtrip)
		if err != nil {
			return fmt.Errorf("evaluate %s: %w", path, err)
		}
		rendered = append(rendered, out...)
	}

	if len(rendered) == 0 {
		return fmt.Errorf("%s: filter produced no output", path)
	}

	return writeFileAtomically(path, []byte(strings.Join(rendered, "\n---\n")+"\n"))
}

// RunFiles runs RunFile against every distinct path in paths
// concurrently (bounded by runtime.NumCPU()), continuing past a
// failure on one file rather than aborting the rest -- matching
// python-yq's own -i behavior of editing every file it was given, each
// with its own result. gojq's compiled *gojq.Code (shared via engine,
// reused across every call) is documented safe to call from
// goroutines, but a *file* is not: if the same path appears more than
// once in paths (e.g. overlapping glob expansion), those occurrences
// are run sequentially, in their original relative order, rather than
// concurrently -- running them concurrently would race on that file's
// own contents (each read-modify-write composes on the previous
// occurrence's output) and silently corrupt it instead of erroring.
// It returns one error per failed path, in the same order as paths
// (not completion order, so the result is deterministic); a nil slice
// means every file succeeded.
func RunFiles(ctx context.Context, paths []string, engine *Engine, opts FormatOptions, slurp, roundtrip bool) []error {
	errs := make([]error, len(paths))

	indicesByPath := make(map[string][]int, len(paths))
	for i, path := range paths {
		indicesByPath[path] = append(indicesByPath[path], i)
	}

	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(runtime.NumCPU())

	for path, indices := range indicesByPath {
		g.Go(func() error {
			// Each occurrence's error is collected into errs[i], not
			// returned to the group, so one file's failure never
			// cancels ctx and aborts sibling paths still in flight.
			for _, i := range indices {
				errs[i] = RunFile(ctx, path, engine, opts, slurp, roundtrip)
			}
			return nil
		})
	}
	_ = g.Wait() // The Go func above always returns nil; nothing to check.

	var result []error
	for _, err := range errs {
		if err != nil {
			result = append(result, err)
		}
	}
	return result
}

// writeFileAtomically writes data to path by writing a temporary file
// in the same directory and renaming it into place, so an interrupted
// write never leaves path truncated or partially written. The
// temporary file is created with path's existing permissions (or 0644
// if path does not yet exist).
func writeFileAtomically(path string, data []byte) error {
	mode := os.FileMode(0o644)
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode()
	}

	tmp, err := os.CreateTemp(filepath.Dir(path), ".yq-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp file for %s: %w", path, err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath) // no-op once the rename below succeeds.

	if _, err := tmp.Write(data); err != nil {
		tmp.Close() //nolint:errcheck // Why: already returning the write error.
		return fmt.Errorf("write temp file for %s: %w", path, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp file for %s: %w", path, err)
	}
	if err := os.Chmod(tmpPath, mode); err != nil {
		return fmt.Errorf("set permissions on temp file for %s: %w", path, err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("rename temp file into place for %s: %w", path, err)
	}
	return nil
}
