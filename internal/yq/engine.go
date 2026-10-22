// Copyright 2026 Outreach Corporation. Licensed under the Apache License 2.0.

// Description: Compiling and running jq filters against decoded YAML
// values via gojq's library API.

package yq

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"math"
	"math/big"
	"os"
	"slices"
	"strconv"

	"github.com/itchyny/gojq"
)

// errJSONTrailingData is returned by parseJSON when raw contains valid
// JSON followed by further, unparsed data.
var errJSONTrailingData = errors.New("invalid JSON: trailing data after value")

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
	slices.Sort(names) // a fixed order for Run to reproduce below.

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
// builtins (arithmetic, length, etc.) can return instead. A result
// containing a NaN or infinite number fails normalization and is
// returned as an error (see ErrNonFiniteNumber), alongside any values
// already emitted before it, the same as a jq-level error above.
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
		norm, err := normalizeValue(v)
		if err != nil {
			return results, fmt.Errorf("evaluate filter %s: %w", e.filter, err)
		}
		results = append(results, norm)
	}
}

// ErrNonFiniteNumber is returned when a filter's result contains a NaN
// or infinite number: neither JSON nor YAML has a literal for either,
// so there is no value FormatValue or Roundtrip could produce.
var ErrNonFiniteNumber = errors.New("result is not a finite number (NaN or Infinity), which has no JSON/YAML representation")

// normalizeValue recursively converts every int, float64, and *big.Int
// in v into json.Number. yq.go's own YAML decoding already produces
// json.Number directly for numbers (see its own doc comment); this
// normalizes the other numeric types gojq's own builtins can return
// instead (e.g. plain int from arithmetic like `+`), which earlier
// caused such a result to fall through format.go's/roundtrip.go's
// number handling and render incorrectly (a bare int was mistaken for
// "not a known value type" and stringified, quoting "11" as a YAML
// string instead of a number). A NaN or infinite float64 is rejected
// rather than normalized: strconv.FormatFloat would otherwise turn it
// into a json.Number holding the literal text "NaN"/"+Inf"/"-Inf",
// which the JSON encoder writes unquoted as invalid JSON, and the YAML
// encoder writes as a plain scalar that decodes back as the string
// "NaN", not a number.
//
// A map or slice with nothing inside it left to convert is returned
// completely unchanged -- the identical value, not a copy -- rather
// than rebuilt. This is not just an efficiency choice: it preserves
// map pointer identity for format.go's/roundtrip.go's KeyOrder, which
// looks up a sub-object's recorded key order by its map's own pointer;
// an object already fully json.Number'd (true of everything
// SplitYAMLDocuments decoded, on any sub-path a filter didn't touch)
// must keep its original pointer or that lookup silently falls back to
// alphabetical order for it. It also matters for data ownership: a
// filter's result can alias a value *Engine doesn't own -- e.g. a
// --arg/--argjson binding returned verbatim (`$extra`), shared across
// every call to Run on the same *Engine, including (a later caller in
// this stack runs several files' filters in parallel against one
// shared engine) concurrently -- so a map or slice actually containing
// something to convert is normalized into a fresh copy rather than
// mutated in place, leaving the original only ever read.
func normalizeValue(v any) (any, error) {
	out, _, err := normalizeValueCOW(v)
	return out, err
}

// normalizeValueCOW is normalizeValue's implementation. Besides the
// normalized value and any error, it reports whether it actually
// converted anything inside v, so a parent map/slice can tell whether
// it needs its own fresh copy or can return v unchanged -- see
// normalizeValue's own doc comment for why that distinction matters.
func normalizeValueCOW(v any) (result any, changed bool, err error) {
	switch val := v.(type) {
	case int:
		return json.Number(strconv.Itoa(val)), true, nil
	case float64:
		if math.IsNaN(val) || math.IsInf(val, 0) {
			return nil, false, fmt.Errorf("%w: %v", ErrNonFiniteNumber, val)
		}
		return json.Number(strconv.FormatFloat(val, 'g', -1, 64)), true, nil
	case *big.Int:
		return json.Number(val.String()), true, nil
	case map[string]any:
		// out stays nil until the first item that actually changes --
		// cloning val lazily, right there, rather than up front. The
		// clone always starts from val itself (not from partial loop
		// state), so it already holds every key's correct value no
		// matter which key triggers it: unvisited keys get their
		// (already correct, since unconverted) original value, and
		// visited-but-unchanged keys keep the value they're cloned
		// with. Only a key whose own item changed is overwritten.
		var out map[string]any
		for k, item := range val {
			norm, itemChanged, err := normalizeValueCOW(item)
			if err != nil {
				return nil, false, err
			}
			if itemChanged {
				if out == nil {
					out = maps.Clone(val)
				}
				out[k] = norm
			}
		}
		if out == nil {
			return val, false, nil
		}
		return out, true, nil
	case []any:
		// Same lazy-clone approach as the map case above.
		var norms []any
		for i, item := range val {
			norm, itemChanged, err := normalizeValueCOW(item)
			if err != nil {
				return nil, false, err
			}
			if itemChanged {
				if norms == nil {
					norms = slices.Clone(val)
				}
				norms[i] = norm
			}
		}
		if norms == nil {
			return val, false, nil
		}
		return norms, true, nil
	default:
		return v, false, nil
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
// way YAML-decoded integers do (see yq.go), rather than becoming
// float64.
func parseJSON(raw string) (any, error) {
	dec := json.NewDecoder(bytes.NewReader([]byte(raw)))
	dec.UseNumber()

	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}
	if dec.More() {
		return nil, errJSONTrailingData
	}
	return v, nil
}
