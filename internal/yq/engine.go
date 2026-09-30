// Copyright 2026 Outreach Corporation. Licensed under the Apache License 2.0.

// Description: Compiling and running jq filters against decoded YAML
// values via gojq's library API.

package yq

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"sort"
	"strconv"

	"github.com/itchyny/gojq"
)

// Engine is a compiled jq filter, ready to run against one or more
// input values.
type Engine struct {
	code   *gojq.Code
	filter string

	// varNames is the fixed order variable values are passed to
	// code.Run/RunWithContext in -- gojq binds them positionally, by
	// the same order given to gojq.WithVariables at compile time, not
	// by name, so Run must reproduce that exact order every time.
	varNames []string
	vars     map[string]any
}

// Compile parses and compiles filter, with vars (--arg/--argjson/
// --slurpfile bindings, keyed by variable name without the leading
// "$") available as jq variables.
func Compile(filter string, vars map[string]any) (*Engine, error) {
	query, err := gojq.Parse(filter)
	if err != nil {
		return nil, fmt.Errorf("parse filter %q: %w", filter, err)
	}

	names := make([]string, 0, len(vars))
	for name := range vars {
		names = append(names, name)
	}
	sort.Strings(names) // a fixed order for Run to reproduce below.

	jqNames := make([]string, len(names))
	for i, name := range names {
		jqNames[i] = "$" + name
	}

	code, err := gojq.Compile(query, gojq.WithVariables(jqNames))
	if err != nil {
		return nil, fmt.Errorf("compile filter %q: %w", filter, err)
	}

	return &Engine{code: code, filter: filter, varNames: names, vars: vars}, nil
}

// Run evaluates the compiled filter against input, returning every
// value it emits, in order. A jq-level error (from the error/0
// builtin, or a runtime type error) is returned as the error, but
// alongside any values already emitted before it -- matching gojq's
// own CLI, which reports such an error without discarding prior
// output.
//
// Every emitted value is normalized (see normalizeValue) before it is
// returned, so every downstream consumer (format.go, roundtrip.go)
// only ever has to handle gojq's decoded-YAML numeric type
// (json.Number), not the several different numeric types gojq's own
// builtins (arithmetic, length, etc.) can return instead.
func (e *Engine) Run(ctx context.Context, input any) ([]any, error) {
	values := make([]any, len(e.varNames))
	for i, name := range e.varNames {
		values[i] = e.vars[name]
	}

	iter := e.code.RunWithContext(ctx, input, values...)
	var results []any
	for {
		v, ok := iter.Next()
		if !ok {
			return results, nil
		}
		if err, ok := v.(error); ok {
			// %s, not %q: a jq filter often contains its own double
			// quotes (e.g. error("boom")), which %q would escape into a
			// confusing double-quoted mess in this message.
			return results, fmt.Errorf("evaluate filter %s: %w", e.filter, err)
		}
		results = append(results, normalizeValue(v))
	}
}

// normalizeValue recursively converts every int, float64, and *big.Int
// in v into json.Number, in place for maps and slices. convert.go's
// own YAML decoding already produces json.Number directly for numbers
// (see its own doc comment); this normalizes the other numeric types
// gojq's own builtins can return instead (e.g. plain int from
// arithmetic like `+`), which earlier caused such a result to fall
// through format.go's/roundtrip.go's number handling and render
// incorrectly (a bare int was mistaken for "not a known value type"
// and stringified, quoting "11" as a YAML string instead of a number).
func normalizeValue(v any) any {
	switch val := v.(type) {
	case int:
		return json.Number(strconv.Itoa(val))
	case float64:
		return json.Number(strconv.FormatFloat(val, 'g', -1, 64))
	case *big.Int:
		return json.Number(val.String())
	case map[string]any:
		for k, item := range val {
			val[k] = normalizeValue(item)
		}
		return val
	case []any:
		for i, item := range val {
			val[i] = normalizeValue(item)
		}
		return val
	default:
		return v
	}
}

// BuildVars converts a yqFlags' --arg/--argjson/--slurpfile bindings
// into the vars map Compile expects. --arg binds a raw string,
// --argjson strictly parses its value as JSON, and --slurpfile reads
// every YAML/JSON document in the named file into a variable bound to
// the array of all of them (matching box.sh:52-53's
// `--slurpfile boxconf <file> '. * {config: $boxconf[0]}'` usage).
func BuildVars(flags *yqFlags) (map[string]any, error) {
	vars := make(map[string]any, len(flags.Arg)+len(flags.ArgJSON)+len(flags.SlurpFile))

	for name, value := range flags.Arg {
		vars[name] = value
	}

	for name, raw := range flags.ArgJSON {
		v, err := parseJSON(raw)
		if err != nil {
			return nil, fmt.Errorf("--argjson %s: %w", name, err)
		}
		vars[name] = v
	}

	for name, path := range flags.SlurpFile {
		b, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("--slurpfile %s: read %s: %w", name, path, err)
		}
		docs, _, err := SplitYAMLDocuments(b)
		if err != nil {
			return nil, fmt.Errorf("--slurpfile %s: decode %s: %w", name, path, err)
		}
		vars[name] = docs
	}

	return vars, nil
}

// parseJSON strictly parses raw as a single JSON value, using
// json.Number for numbers so integers survive round-trips the same
// way YAML-decoded integers do (see convert.go), rather than becoming
// float64.
func parseJSON(raw string) (any, error) {
	dec := json.NewDecoder(bytes.NewReader([]byte(raw)))
	dec.UseNumber()

	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}
	if dec.More() {
		return nil, fmt.Errorf("invalid JSON: trailing data after value")
	}
	return v, nil
}
