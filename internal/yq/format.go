// Copyright 2026 Outreach Corporation. Licensed under the Apache License 2.0.

// Description: Rendering a gojq result value as JSON or YAML, honoring
// default key order (falling back to alphabetical under -S), raw
// string output, compact JSON, ASCII-escaped output, and indentless
// YAML sequences.

package yq

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	yaml "github.com/itchyny/go-yaml"
)

// FormatOptions controls how FormatValue renders a single gojq result
// value.
type FormatOptions struct {
	// Raw unwraps a top-level string result instead of quoting it as
	// JSON/YAML; a non-string result is unaffected (box.sh's `-r .` on
	// a mapping must still emit JSON).
	Raw bool

	// YAML renders the value as YAML instead of JSON.
	YAML bool

	// Compact disables pretty-printing for JSON output. It has no
	// effect on YAML output, which has no compact form.
	Compact bool

	// SortKeys skips Order and always emits object keys
	// alphabetically.
	SortKeys bool

	// ASCII escapes non-ASCII characters in JSON string output. It has
	// no effect on YAML output.
	ASCII bool

	// Indentless renders YAML sequences without the extra indent level
	// before "-" (github.com/itchyny/go-yaml's CompactSeqIndent). It
	// has no effect on JSON output.
	Indentless bool

	// Order records each decoded object's original key order (see
	// convert.go). A value's keys are emitted in this order by
	// default; SortKeys overrides it, and a map missing from Order (or
	// whose key count no longer matches) falls back to alphabetical
	// regardless -- see KeyOrder.order.
	Order KeyOrder
}

// FormatValue renders v per opts. The returned bytes have no trailing
// newline; the caller joins multiple values as it sees fit (e.g. -i's
// "---"-separated multi-doc stream).
func FormatValue(v any, opts FormatOptions) ([]byte, error) {
	if opts.Raw {
		if s, ok := v.(string); ok {
			return []byte(s), nil
		}
	}

	if opts.YAML {
		return formatYAML(v, opts)
	}
	return formatJSON(v, opts)
}

// orderedKeys returns m's keys in the order Order records for it, or
// alphabetically if opts.SortKeys is set or no recorded order applies
// (KeyOrder.order already accounts for a stale or missing entry).
// Shared by both the JSON and YAML encoders below so "default key
// order vs. -S" behaves identically for either output format.
func orderedKeys(m map[string]any, opts FormatOptions) []string {
	if !opts.SortKeys {
		if keys, ok := opts.Order.order(m); ok {
			return keys
		}
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// formatJSON renders v as JSON. It does not use encoding/json.Marshal
// for objects, since that always sorts map keys alphabetically with no
// way to disable it -- incompatible with preserving default key order.
func formatJSON(v any, opts FormatOptions) ([]byte, error) {
	var buf bytes.Buffer
	if err := encodeJSON(&buf, v, opts, 0); err != nil {
		return nil, fmt.Errorf("encode json: %w", err)
	}
	return buf.Bytes(), nil
}

// encodeJSON writes v to buf at the given indent depth (ignored when
// opts.Compact is set). v's numbers are always json.Number here:
// Engine.Run normalizes every other numeric type gojq can emit (plain
// int from arithmetic, etc.) into json.Number before returning, so
// this (like yamlNodeFor below) only has to handle the one type.
func encodeJSON(buf *bytes.Buffer, v any, opts FormatOptions, depth int) error {
	switch val := v.(type) {
	case nil:
		buf.WriteString("null")
	case bool:
		if val {
			buf.WriteString("true")
		} else {
			buf.WriteString("false")
		}
	case json.Number:
		buf.WriteString(val.String())
	case string:
		encodeJSONString(buf, val, opts.ASCII)
	case []any:
		return encodeJSONArray(buf, val, opts, depth)
	case map[string]any:
		return encodeJSONObject(buf, val, opts, depth)
	default:
		// Not one of gojq's own output value types; fall back to the
		// standard encoder rather than erroring.
		b, err := json.Marshal(val)
		if err != nil {
			return fmt.Errorf("encode value of type %T: %w", val, err)
		}
		buf.Write(b)
	}
	return nil
}

func encodeJSONArray(buf *bytes.Buffer, arr []any, opts FormatOptions, depth int) error {
	if len(arr) == 0 {
		buf.WriteString("[]")
		return nil
	}
	buf.WriteByte('[')
	for i, item := range arr {
		if i > 0 {
			buf.WriteByte(',')
		}
		jsonNewlineIndent(buf, opts, depth+1)
		if err := encodeJSON(buf, item, opts, depth+1); err != nil {
			return err
		}
	}
	jsonNewlineIndent(buf, opts, depth)
	buf.WriteByte(']')
	return nil
}

func encodeJSONObject(buf *bytes.Buffer, m map[string]any, opts FormatOptions, depth int) error {
	if len(m) == 0 {
		buf.WriteString("{}")
		return nil
	}
	buf.WriteByte('{')
	for i, k := range orderedKeys(m, opts) {
		if i > 0 {
			buf.WriteByte(',')
		}
		jsonNewlineIndent(buf, opts, depth+1)
		encodeJSONString(buf, k, opts.ASCII)
		buf.WriteByte(':')
		if !opts.Compact {
			buf.WriteByte(' ')
		}
		if err := encodeJSON(buf, m[k], opts, depth+1); err != nil {
			return err
		}
	}
	jsonNewlineIndent(buf, opts, depth)
	buf.WriteByte('}')
	return nil
}

func jsonNewlineIndent(buf *bytes.Buffer, opts FormatOptions, depth int) {
	if opts.Compact {
		return
	}
	buf.WriteByte('\n')
	buf.WriteString(strings.Repeat("  ", depth))
}

// encodeJSONString writes s as a quoted JSON string, escaping control
// characters and, under ascii, every non-ASCII rune as \uXXXX (with a
// surrogate pair for runes outside the basic multilingual plane).
func encodeJSONString(buf *bytes.Buffer, s string, ascii bool) {
	buf.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			buf.WriteString(`\"`)
		case '\\':
			buf.WriteString(`\\`)
		case '\n':
			buf.WriteString(`\n`)
		case '\r':
			buf.WriteString(`\r`)
		case '\t':
			buf.WriteString(`\t`)
		default:
			switch {
			case r < 0x20:
				fmt.Fprintf(buf, `\u%04x`, r)
			case r < 0x80 || !ascii:
				buf.WriteRune(r)
			case r > 0xFFFF:
				r -= 0x10000
				fmt.Fprintf(buf, `\u%04x\u%04x`, 0xD800+(r>>10), 0xDC00+(r&0x3FF))
			default:
				fmt.Fprintf(buf, `\u%04x`, r)
			}
		}
	}
	buf.WriteByte('"')
}

// formatYAML renders v as YAML by first converting it into a
// github.com/itchyny/go-yaml Node tree (in opts's key order), then
// encoding that Node directly -- encoding a *Node bypasses the
// library's generic map encoding, which (like encoding/json) always
// sorts keys alphabetically.
func formatYAML(v any, opts FormatOptions) ([]byte, error) {
	return encodeYAMLNode(yamlNodeFor(v, opts), opts.Indentless)
}

// encodeYAMLNode encodes node (built either by yamlNodeFor, for plain
// default/-S output, or by roundtrip.go's reconcile, for -Y) as YAML.
// Shared by formatYAML and Roundtrip so the encoder setup (2-space
// indent matching this repo's own YAML files, --indentless-lists,
// trailing-newline trimming, error wrapping) stays in one place.
func encodeYAMLNode(node *yaml.Node, indentless bool) ([]byte, error) {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if indentless {
		enc.CompactSeqIndent()
	}
	if err := enc.Encode(node); err != nil {
		return nil, fmt.Errorf("encode yaml: %w", err)
	}
	if err := enc.Close(); err != nil {
		return nil, fmt.Errorf("encode yaml: %w", err)
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}

// yamlNodeFor converts v into a plain-style Node tree, ordering object
// keys per opts. v's numbers are always json.Number here, for the same
// reason encodeJSON's doc comment gives.
func yamlNodeFor(v any, opts FormatOptions) *yaml.Node {
	switch val := v.(type) {
	case nil:
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!null", Value: "null"}
	case bool:
		n := &yaml.Node{Kind: yaml.ScalarNode}
		if val {
			n.Value = "true"
		} else {
			n.Value = "false"
		}
		return n
	case json.Number:
		return &yaml.Node{Kind: yaml.ScalarNode, Value: val.String()}
	case string:
		n := &yaml.Node{}
		n.SetString(val)
		return n
	case []any:
		seq := &yaml.Node{Kind: yaml.SequenceNode}
		for _, item := range val {
			seq.Content = append(seq.Content, yamlNodeFor(item, opts))
		}
		return seq
	case map[string]any:
		mapping := &yaml.Node{Kind: yaml.MappingNode}
		for _, k := range orderedKeys(val, opts) {
			keyNode := &yaml.Node{}
			keyNode.SetString(k)
			mapping.Content = append(mapping.Content, keyNode, yamlNodeFor(val[k], opts))
		}
		return mapping
	default:
		n := &yaml.Node{}
		n.SetString(fmt.Sprint(val))
		return n
	}
}
