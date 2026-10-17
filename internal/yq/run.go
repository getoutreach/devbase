// Copyright 2026 Outreach Corporation. Licensed under the Apache License 2.0.

// Description: Top-level orchestration for the `devbase yq` CLI
// subcommand: parsing argv, compiling the filter, and running it
// against stdin/files or editing files in place.

package yq

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"

	"github.com/getoutreach/gobox/pkg/app"
	yaml "github.com/itchyny/go-yaml"
)

// Main runs the yq subcommand end to end against args (everything
// after "yq" on the command line, unparsed -- see the design plan's
// "Flag parsing decision" for why devbase yq parses its own flags
// rather than using cli/v3's flag handling), reading from stdin when
// no file operands are given, and returns the process exit code to
// use. It is the single entry point cmd/devbase/yq.go calls, so that
// package can stay thin CLI wiring: everything about how a flag or a
// filter behaves lives here in the yq package instead.
func Main(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	flags, rest, err := parseYqFlags(args)
	if err != nil {
		return fail(stderr, "%v", err)
	}
	if err := flags.Validate(); err != nil {
		return fail(stderr, "%v", err)
	}

	// -V/--version works standalone, without a filter argument, the
	// same way gojq's and jq's own --version do. It reports the
	// devbase binary's own version (app.Version, the same ldflags-
	// stamped value `devbase --version` reports at the top level) --
	// not gojq's, python-yq's, or mikefarah go-yq's, per the design
	// plan's stance of not imitating another dialect's version string.
	if flags.Version {
		fmt.Fprintf(stdout, "devbase yq (devbase %s)\n", app.Version)
		return 0
	}

	if len(rest) == 0 {
		return fail(stderr, "missing filter argument")
	}
	filter, files := rest[0], rest[1:]

	if flags.InPlace && len(files) == 0 {
		return fail(stderr, "-i/--in-place requires at least one file, not stdin")
	}

	vars, err := BuildVars(flags)
	if err != nil {
		return fail(stderr, "%v", err)
	}

	engine, err := Compile(filter, vars)
	if err != nil {
		return fail(stderr, "%v", err)
	}

	opts := FormatOptions{
		Raw:        flags.RawOutput,
		YAML:       flags.YAMLOutput || flags.YAMLRoundtrip, // -Y implies -y.
		Compact:    flags.CompactOutput,
		SortKeys:   flags.SortKeys,
		ASCII:      flags.ASCIIOutput,
		Indentless: flags.IndentlessLists,
	}

	if flags.InPlace {
		return runInPlace(ctx, files, engine, opts, flags.Slurp, flags.YAMLRoundtrip, stderr)
	}
	return runToStdout(ctx, stdin, stdout, stderr, files, flags.Slurp, flags.YAMLRoundtrip, engine, opts)
}

// fail writes a "yq: "-prefixed message to stderr (the one place that
// prefix gets added, so every error site above stays uniform whether
// it wraps an error value or a literal message) and returns the exit
// code Main should return for it.
func fail(stderr io.Writer, format string, args ...any) int {
	fmt.Fprintf(stderr, "yq: "+format+"\n", args...)
	return 1
}

// runInPlace edits every file in files independently via RunFiles,
// reporting every failure without letting one abort the rest.
func runInPlace(ctx context.Context, files []string, engine *Engine, opts FormatOptions, slurp, roundtrip bool, stderr io.Writer) int {
	errs := RunFiles(ctx, files, engine, opts, slurp, roundtrip)
	for _, err := range errs {
		fmt.Fprintf(stderr, "yq: %v\n", err)
	}
	if len(errs) > 0 {
		return 1
	}
	return 0
}

// runToStdout reads input from stdin (when files is empty) or the
// given files concatenated, runs the filter against each decoded
// document (or, under slurp, once against the array of all of them),
// and writes every formatted result to stdout. Consecutive YAML
// results get a "---" document separator between them, matching
// gojq's own --yaml-output behavior; JSON results need no separator,
// since each is already self-delimiting.
func runToStdout(ctx context.Context, stdin io.Reader, stdout, stderr io.Writer,
	files []string, slurp, roundtrip bool, engine *Engine, opts FormatOptions,
) int {
	b, err := readInput(stdin, files)
	if err != nil {
		fmt.Fprintf(stderr, "yq: %v\n", err)
		return 1
	}

	docs, nodes, order, err := SplitYAMLDocumentsWithNodes(b)
	if err != nil {
		fmt.Fprintf(stderr, "yq: %v\n", err)
		return 1
	}
	opts.Order = order

	if slurp {
		// Slurped input has no single original document for -Y to
		// reconcile against; fall back to plain formatting for it.
		docs, nodes, roundtrip = []any{docs}, []*yaml.Node{nil}, false
	}

	hasError := false
	first := true
	for i, doc := range docs {
		// renderDocument may return both a non-empty rendered and a
		// non-nil err: print whatever was rendered before reporting
		// the error, matching Engine.Run's own contract of keeping
		// every value emitted before a jq-level error (e.g. `1,
		// error("boom")` still prints 1 first, same as real jq/gojq).
		rendered, err := renderDocument(ctx, doc, nodes[i], engine, opts, roundtrip)
		for _, out := range rendered {
			if opts.YAML && !first {
				fmt.Fprintln(stdout, "---")
			}
			fmt.Fprintln(stdout, out)
			first = false
		}
		if err != nil {
			fmt.Fprintf(stderr, "yq: %v\n", err)
			hasError = true
		}
	}

	if hasError {
		return 1
	}
	return 0
}

// renderDocument runs engine against doc and renders each emitted
// value, either via FormatValue or, under roundtrip (with a non-nil
// node), via Roundtrip against doc's original parse tree. If
// engine.Run itself returns an error, renderDocument still renders
// every value emitted before that error and returns both the
// rendered strings so far and the error, rather than discarding
// them -- matching Engine.Run's own documented contract.
func renderDocument(ctx context.Context, doc any, node *yaml.Node, engine *Engine, opts FormatOptions, roundtrip bool) ([]string, error) {
	results, runErr := engine.Run(ctx, doc)

	rendered := make([]string, 0, len(results))
	for _, v := range results {
		var out []byte
		var err error
		if roundtrip && node != nil {
			out, err = Roundtrip(node, v, opts)
		} else {
			out, err = FormatValue(v, opts)
		}
		if err != nil {
			return rendered, err
		}
		rendered = append(rendered, string(out))
	}
	return rendered, runErr
}

// readInput returns stdin's contents when files is empty, or every
// named file's contents concatenated (in argument order) otherwise,
// each as its own YAML document (joined by a "---" separator, not a
// bare newline, so e.g. two files each with a top-level "name:" key
// decode as two documents instead of one with a duplicate-key error).
func readInput(stdin io.Reader, files []string) ([]byte, error) {
	if len(files) == 0 {
		b, err := io.ReadAll(stdin)
		if err != nil {
			return nil, fmt.Errorf("read stdin: %w", err)
		}
		return b, nil
	}

	var buf bytes.Buffer
	for i, path := range files {
		if i > 0 {
			buf.WriteString("\n---\n")
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", path, err)
		}
		buf.Write(b)
	}
	return buf.Bytes(), nil
}
