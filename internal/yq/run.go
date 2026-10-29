// Copyright 2026 Outreach Corporation. Licensed under the Apache License 2.0.

// Description: Top-level orchestration for the `devbase yq` and `devbase tomlq` CLI
// subcommand: parsing argv, compiling the filter, and running it
// against stdin/files or editing files in place.

package yq

import (
	"context"
	"fmt"
	"io"
	"maps"
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
	return run(ctx, yamlInput, args, stdin, stdout, stderr)
}

// MainTOML is Main for the `devbase tomlq` subcommand: the same filter
// engine and flags, but reading TOML instead of YAML, and always
// writing JSON.
func MainTOML(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	return run(ctx, tomlInput, args, stdin, stdout, stderr)
}

// decodeFunc decodes one input's bytes into its documents, the parse
// tree of each (nil when the format has none, so -Y cannot apply), and
// the key order of every object decoded.
type decodeFunc func(b []byte) ([]any, []*yaml.Node, KeyOrder, error)

// inputFormat is what differs between `devbase yq` and `devbase tomlq`.
type inputFormat struct {
	// name is the command name used to prefix error messages.
	name   string
	decode decodeFunc
	// validate, when set, runs before Validate and rejects flags this
	// format cannot honor.
	validate func(*yqFlags) error
}

//nolint:gochecknoglobals // Why: read-only lookup tables.
var (
	// yamlInput is the input format of `devbase yq`.
	yamlInput = inputFormat{name: "yq", decode: SplitYAMLDocumentsWithNodes}

	// tomlInput is the input format of `devbase tomlq`.
	tomlInput = inputFormat{name: "tomlq", decode: decodeTOMLInput, validate: validateTOML}
)

// run is Main and MainTOML's shared implementation.
func run(ctx context.Context, in inputFormat, args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	flags, rest, err := parseYqFlags(args)
	if err != nil {
		return fail(stderr, in.name, "%v", err)
	}
	if in.validate != nil {
		if err := in.validate(flags); err != nil {
			return fail(stderr, in.name, "%v", err)
		}
	}
	if err := flags.Validate(); err != nil {
		return fail(stderr, in.name, "%v", err)
	}

	// -V/--version works standalone, without a filter argument, the
	// same way gojq's and jq's own --version do. It reports the
	// devbase binary's own version (app.Version, the same ldflags-
	// stamped value `devbase --version` reports at the top level) --
	// not gojq's, python-yq's, or mikefarah go-yq's, per the design
	// plan's stance of not imitating another dialect's version string.
	if flags.Version {
		fmt.Fprintf(stdout, "devbase %s (devbase %s)\n", in.name, app.Version)
		return 0
	}

	if len(rest) == 0 {
		return fail(stderr, in.name, "missing filter argument")
	}
	filter, files := rest[0], rest[1:]

	if flags.InPlace && len(files) == 0 {
		return fail(stderr, in.name, "-i/--in-place requires at least one file, not stdin")
	}

	vars, err := BuildVars(flags)
	if err != nil {
		return fail(stderr, in.name, "%v", err)
	}

	engine, err := Compile(filter, vars)
	if err != nil {
		return fail(stderr, in.name, "%v", err)
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
		return runInPlace(ctx, in.name, files, engine, opts, flags.Slurp, flags.YAMLRoundtrip, stderr)
	}
	return runToStdout(ctx, in, stdin, stdout, stderr, files, flags.Slurp, flags.YAMLRoundtrip, engine, opts)
}

// fail writes a message prefixed with the command name to stderr (the
// one place that prefix gets added, so every error site above stays
// uniform whether it wraps an error value or a literal message) and
// returns the exit code Main should return for it.
func fail(stderr io.Writer, name, format string, args ...any) int {
	fmt.Fprintf(stderr, name+": "+format+"\n", args...)
	return 1
}

// runInPlace edits every file in files independently via RunFiles,
// reporting every failure without letting one abort the rest.
func runInPlace(ctx context.Context, name string, files []string, engine *Engine, opts FormatOptions,
	slurp, roundtrip bool, stderr io.Writer,
) int {
	errs := RunFiles(ctx, files, engine, opts, slurp, roundtrip)
	for _, err := range errs {
		fmt.Fprintf(stderr, "%s: %v\n", name, err)
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
func runToStdout(ctx context.Context, in inputFormat, stdin io.Reader, stdout, stderr io.Writer,
	files []string, slurp, roundtrip bool, engine *Engine, opts FormatOptions,
) int {
	docs, nodes, order, err := readDocuments(stdin, files, in.decode)
	if err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", in.name, err)
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
			fmt.Fprintf(stderr, "%s: %v\n", in.name, err)
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

// decodeFile reads and decodes path's YAML document(s), wrapping
// either error with path for context. Shared by RunFile (inplace.go,
// one file at a time, for -i) and readDocuments below (several file
// operands combined into one stream, for plain stdout output).
func decodeFile(path string, decode decodeFunc) (docs []any, nodes []*yaml.Node, order KeyOrder, err error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("read %s: %w", path, err)
	}
	docs, nodes, order, err = decode(b)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("decode %s: %w", path, err)
	}
	return docs, nodes, order, nil
}

// readDocuments decodes stdin's contents when files is empty, or every
// named file's own YAML document(s), decoded independently and
// combined in argument order, otherwise. Each file is decoded on its
// own (not concatenated with the others behind an injected "---"
// separator first) so a file that already begins with its own "---"
// marker can't collide with that separator and produce a spurious
// empty document between the two files. Merging every file's order
// into one map can never collide: order's keys are map pointers (see
// KeyOrder's own doc comment), and every decoded map stays referenced,
// through the growing docs slice, for the rest of this function, so no
// two of them can ever share an address.
func readDocuments(stdin io.Reader, files []string, decode decodeFunc) (docs []any, nodes []*yaml.Node, order KeyOrder, err error) {
	if len(files) == 0 {
		b, err := io.ReadAll(stdin)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("read stdin: %w", err)
		}
		return decode(b)
	}

	order = make(KeyOrder)
	for _, path := range files {
		fileDocs, fileNodes, fileOrder, err := decodeFile(path, decode)
		if err != nil {
			return nil, nil, nil, err
		}
		docs = append(docs, fileDocs...)
		nodes = append(nodes, fileNodes...)
		maps.Copy(order, fileOrder)
	}
	return docs, nodes, order, nil
}
