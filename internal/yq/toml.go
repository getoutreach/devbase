// Copyright 2026 Outreach Corporation. Licensed under the Apache License 2.0.

// Description: TOML decoding into the plain values gojq operates on,
// for `devbase tomlq`.

package yq

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"time"

	yaml "github.com/itchyny/go-yaml"
	toml "github.com/pelletier/go-toml/v2"
)

// decodeTOML decodes the TOML document in b into the plain value shape
// gojq expects (see decodeNode for YAML's equivalent). Integers and
// floats become json.Number, and dates and times become strings: JSON
// has no date type, and gojq cannot operate on a time.Time. The result
// is always a map, as TOML's top level is always a table.
//
// Unlike YAML decoding, DecodeTOML records no key order, so a filter's
// object results are written with their keys sorted alphabetically.
func decodeTOML(b []byte) (any, error) {
	var v map[string]any
	if err := toml.Unmarshal(b, &v); err != nil {
		return nil, fmt.Errorf("decode toml: %w", err)
	}
	return normalizeTOML(v), nil
}

// normalizeTOML recursively converts the numeric and date/time values
// go-toml decodes into the types the rest of this package expects. It
// edits v's maps and slices in place, as DecodeTOML owns the tree.
func normalizeTOML(v any) any {
	switch val := v.(type) {
	case int64:
		return json.Number(strconv.FormatInt(val, 10))
	case float64:
		// NaN and infinity stay float64, so Engine.Run rejects them
		// with ErrNonFiniteNumber instead of this wrapping invalid
		// JSON text (see fixupNumberScalars for the YAML equivalent).
		if math.IsNaN(val) || math.IsInf(val, 0) {
			return val
		}
		return json.Number(strconv.FormatFloat(val, 'g', -1, 64))
	case time.Time:
		return val.Format(time.RFC3339Nano)
	case fmt.Stringer: // go-toml's LocalDate, LocalTime, and LocalDateTime, which have no zone.
		return val.String()
	case map[string]any:
		for k, item := range val {
			val[k] = normalizeTOML(item)
		}
		return val
	case []any:
		for i, item := range val {
			val[i] = normalizeTOML(item)
		}
		return val
	default:
		return v
	}
}

// decodeTOMLInput is the decodeFunc for TOML: one document per input,
// with no YAML parse tree and no recorded key order.
func decodeTOMLInput(b []byte) ([]any, []*yaml.Node, KeyOrder, error) {
	v, err := decodeTOML(b)
	if err != nil {
		return nil, nil, nil, err
	}
	return []any{v}, []*yaml.Node{nil}, KeyOrder{}, nil
}

// errTOMLUnsupportedFlag is wrapped into the error validateTOML
// returns for a flag that has no meaning when reading TOML.
var errTOMLUnsupportedFlag = errors.New("not supported by devbase tomlq, which reads TOML and writes JSON")

// validateTOML returns an error for the first flag set in f that
// devbase tomlq cannot honor. It runs before yqFlags.Validate, so -t
// and -T are reported as tomlq's own, not as a missing yq feature.
func validateTOML(f *yqFlags) error {
	unsupported := []struct {
		name string
		set  bool
	}{
		{"-y/--yaml-output", f.YAMLOutput},
		{"-Y/--yaml-roundtrip", f.YAMLRoundtrip},
		{"--indentless-lists", f.IndentlessLists},
		{"-i/--in-place", f.InPlace},
		{"-t/--toml-output", f.TOMLOutput},
		{"-T/--toml-roundtrip", f.TOMLRoundtrip},
	}
	for _, u := range unsupported {
		if u.set {
			return fmt.Errorf("%s is %w", u.name, errTOMLUnsupportedFlag)
		}
	}
	return nil
}
