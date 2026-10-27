// Copyright 2026 Outreach Corporation. Licensed under the Apache License 2.0.

// Description: TOML decoding into the plain values gojq operates on,
// for `devbase tomlq`.

package yq

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"reflect"
	"strconv"
	"time"

	toml "github.com/BurntSushi/toml"
	yaml "github.com/itchyny/go-yaml"
)

// BurntSushi/toml decodes every TOML date and time into a time.Time and
// names the location of the zoneless forms after them.
const (
	// tomlLocalDateTime is the location name of a date and time with no offset.
	tomlLocalDateTime = "datetime-local"

	// tomlLocalDate is the location name of a date alone.
	tomlLocalDate = "date-local"

	// tomlLocalTime is the location name of a time alone.
	tomlLocalTime = "time-local"
)

// decodeTOML decodes the TOML document in b into the plain value shape
// gojq expects (see decodeNode for YAML's equivalent), along with the
// KeyOrder of every table decoded. Integers and floats become
// json.Number, and dates and times become strings: JSON has no date
// type, and gojq cannot operate on a time.Time. The value is always a
// map, as TOML's top level is always a table.
func decodeTOML(b []byte) (any, KeyOrder, error) {
	v := map[string]any{}
	md, err := toml.Decode(string(b), &v)
	if err != nil {
		return nil, nil, fmt.Errorf("decode toml: %w", err)
	}

	// Record order only once v is in its final shape: KeyOrder is keyed
	// by map pointer, and normalizeTOML keeps every map but replaces
	// every array of tables.
	root := normalizeTOML(v)
	order := KeyOrder{}
	recordTOMLOrder(root, newTOMLOrder(&md), order)
	return root, order, nil
}

// normalizeTOML recursively converts the numeric, date/time, and
// array-of-tables values BurntSushi/toml decodes into the types the
// rest of this package expects. It edits v's maps in place, as
// decodeTOML owns the tree.
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
		return json.Number(formatTOMLFloat(val))
	case time.Time:
		return formatTOMLTime(val)
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
	case []map[string]any:
		// An array of tables: gojq only accepts []any.
		out := make([]any, len(val))
		for i, item := range val {
			out[i] = normalizeTOML(item)
		}
		return out
	default:
		return v
	}
}

// formatTOMLFloat writes f in plain decimal form, and in exponent form
// only below 1e-6 or from 1e21, as JSON.stringify does. strconv's
// default form switches to exponent form from 1e6, which would print
// 1500000.5 as 1.5000005e+06.
func formatTOMLFloat(f float64) string {
	if abs := math.Abs(f); abs != 0 && (abs < 1e-6 || abs >= 1e21) {
		return strconv.FormatFloat(f, 'g', -1, 64)
	}
	return strconv.FormatFloat(f, 'f', -1, 64)
}

// formatTOMLTime renders t as a date, time, or datetime string. A value
// with an offset is written in RFC 3339, so "+00:00" becomes "Z".
func formatTOMLTime(t time.Time) string {
	switch t.Location().String() {
	case tomlLocalDateTime:
		return t.Format("2006-01-02T15:04:05.999999999")
	case tomlLocalDate:
		return t.Format("2006-01-02")
	case tomlLocalTime:
		return t.Format("15:04:05.999999999")
	default:
		return t.Format(time.RFC3339Nano)
	}
}

// tomlTable is the keys, in document order, that one occurrence of a
// TOML table defines.
type tomlTable struct {
	keys []string
	kids map[string]*tomlPath
}

func newTOMLTable() *tomlTable {
	return &tomlTable{kids: map[string]*tomlPath{}}
}

// tomlPath is every occurrence of one key path in the document: one
// for most keys, but one per element for an array of tables.
type tomlPath struct {
	tables []*tomlTable

	// arrayOfTables is set for a path written as "[[path]]", as opposed
	// to an array of inline tables.
	arrayOfTables bool
}

// last returns p's most recent occurrence, adding one if the document
// never named p itself (as with the "a" of a dotted key "a.b = 1").
func (p *tomlPath) last() *tomlTable {
	if len(p.tables) == 0 {
		p.tables = append(p.tables, newTOMLTable())
	}
	return p.tables[len(p.tables)-1]
}

// newTOMLOrder builds, from md's list of every key path in document
// order, the tree recordTOMLOrder reads each table's key order from. A
// key inside an inline table in an array is listed without saying which
// element it belongs to, so those tables have no order to read.
func newTOMLOrder(md *toml.MetaData) *tomlPath {
	root := newTOMLTable()
	for _, key := range md.Keys() {
		table := root
		for i, segment := range key {
			path, ok := table.kids[segment]
			if !ok {
				path = &tomlPath{}
				table.kids[segment] = path
				table.keys = append(table.keys, segment)
			}
			if i == len(key)-1 {
				path.tables = append(path.tables, newTOMLTable())
				path.arrayOfTables = md.Type(key...) == "ArrayHash"
			} else {
				table = path.last()
			}
		}
	}
	return &tomlPath{tables: []*tomlTable{root}}
}

// recordTOMLOrder walks v, a value decodeTOML produced, together with
// the tomlPath for its position, recording the key order of every map
// into order. A table a document defines in several places (like
// "[a.b]" before "[a]") has its keys joined in document order. A map
// missing a recorded order is left to fall back to alphabetical order
// (see KeyOrder.order).
func recordTOMLOrder(v any, path *tomlPath, order KeyOrder) {
	switch val := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(val))
		seen := make(map[string]bool, len(val))
		for _, table := range path.tables {
			for _, k := range table.keys {
				if _, ok := val[k]; ok && !seen[k] {
					seen[k] = true
					keys = append(keys, k)
				}
			}
		}
		order[reflect.ValueOf(val).Pointer()] = keys

		for k, item := range val {
			for _, table := range path.tables {
				if kid, ok := table.kids[k]; ok {
					recordTOMLOrder(item, kid, order)
					break
				}
			}
		}
	case []any:
		// Only an array of tables has one occurrence per element.
		if !path.arrayOfTables || len(path.tables) != len(val) {
			return
		}
		for i, item := range val {
			recordTOMLOrder(item, &tomlPath{tables: path.tables[i : i+1]}, order)
		}
	}
}

// decodeTOMLInput is the decodeFunc for TOML: one document per input,
// with no YAML parse tree.
func decodeTOMLInput(b []byte) ([]any, []*yaml.Node, KeyOrder, error) {
	v, order, err := decodeTOML(b)
	if err != nil {
		return nil, nil, nil, err
	}
	return []any{v}, []*yaml.Node{nil}, order, nil
}

// errTOMLUnsupportedFlag is wrapped into the error validateTOML
// returns for a flag that has no meaning when reading TOML.
var errTOMLUnsupportedFlag = errors.New("not supported by devbase tomlq, which reads TOML and writes JSON")

// validateTOML returns an error for the first flag set in f that
// devbase tomlq cannot honor: the YAML and XML flags, which can never
// apply to TOML input. It runs before yqFlags.Validate, so these are
// reported as tomlq's own, not as a missing yq feature.
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
		{"-F/--yaml-frontmatter", f.YAMLFrontmatter},
		{"-w/--width", f.Width != nil},
		{"--explicit-start", f.ExplicitStart},
		{"--explicit-end", f.ExplicitEnd},
		{"--no-expand-aliases", f.NoExpandAliases},
		{"--max-expansion-factor", f.MaxExpansionFactor != nil},
		{"--yaml-output-grammar-version/--yml-out-ver", f.YAMLOutputGrammarVersion != ""},
		{"-x/--xml-output", f.XMLOutput},
		{"--xml-input", f.XMLInput},
	}
	for _, u := range unsupported {
		if u.set {
			return fmt.Errorf("%s is %w", u.name, errTOMLUnsupportedFlag)
		}
	}
	return nil
}
