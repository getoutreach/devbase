// Copyright 2026 Outreach Corporation. Licensed under the Apache License 2.0.

// Description: -Y/--yaml-roundtrip: comment/anchor/style-preserving
// output via path-based reconciliation against the original parse
// tree.

package yq

import (
	"encoding/json"
	"reflect"
	"sort"

	yaml "github.com/itchyny/go-yaml"
)

// Roundtrip renders result (one gojq output value for the document
// original was decoded from) as YAML, reusing original's comments,
// anchors, and style wherever result's value at a given path equals
// original's value there. opts.SortKeys and opts.Indentless apply the
// same way they do to plain (non -Y) output; opts.Order is not
// consulted, since path-based reconciliation (below) is its own,
// separate key-order mechanism.
//
// This is deliberately a different mechanism from the pointer-based
// KeyOrder table default (non -Y) output uses (see convert.go): jq's
// `*` merge operator allocates a brand-new map even for keys it left
// untouched, so pointer identity finds nothing for that case (the
// real production use this exists to serve merges a comment-bearing
// document with `. * {someKey: ...}`). Matching by path and value
// equality instead correctly preserves comments on every key a merge
// or filter did not touch, and drops them only where the value
// actually changed.
//
// This is best-effort, not pixel-perfect like ruamel.yaml's roundtrip
// mode: a reordered key, a deleted-then-recreated key, or a filter
// that deeply restructures a subtree still loses that subtree's
// comments. That is an accepted, disclosed tradeoff, not a hidden gap.
func Roundtrip(original *yaml.Node, result any, opts FormatOptions) ([]byte, error) {
	node := reconcile(original, result, opts)
	return encodeYAMLNode(node, opts.Indentless)
}

// reconcile returns a Node rendering result: original itself (with
// every comment, anchor, and style intact) when result's value
// matches original's exactly, a recursively reconciled Node when both
// are mappings or both are sequences, or a plain Node (no
// comments/anchor/style to reuse) when result is new or original's
// Kind doesn't match result's shape at all.
func reconcile(original *yaml.Node, result any, opts FormatOptions) *yaml.Node {
	if original == nil {
		return yamlNodeFor(result, opts)
	}
	original = resolveAlias(original)

	switch val := result.(type) {
	case map[string]any:
		if original.Kind == yaml.MappingNode {
			return reconcileMapping(original, val, opts)
		}
	case []any:
		if original.Kind == yaml.SequenceNode {
			return reconcileSequence(original, val, opts)
		}
	default:
		if original.Kind == yaml.ScalarNode {
			return reconcileScalar(original, result)
		}
	}
	// A type mismatch between original and result (a filter replaced
	// a mapping with a scalar, etc.): nothing to preserve.
	return yamlNodeFor(result, opts)
}

// resolveAlias follows an AliasNode to the node it points to, so
// reconciliation works against the anchor's own real content.
func resolveAlias(n *yaml.Node) *yaml.Node {
	for n.Kind == yaml.AliasNode && n.Alias != nil {
		n = n.Alias
	}
	return n
}

// reconcileScalar reuses original verbatim (comments, anchor, style,
// and position included) when its decoded value equals result;
// otherwise it builds a fresh scalar Node for result but keeps
// original's comments and anchor, so a scalar edit like `.foo = "x"`
// still preserves any comment on foo.
func reconcileScalar(original *yaml.Node, result any) *yaml.Node {
	if valuesEqual(decodeScalarValue(original), result) {
		return original
	}
	n := yamlNodeFor(result, FormatOptions{})
	n.HeadComment, n.LineComment, n.FootComment, n.Anchor = original.HeadComment, original.LineComment, original.FootComment, original.Anchor
	return n
}

// decodeScalarValue decodes original into the same plain value shape
// convert.go's decodeNode produces, so it can be compared against a
// gojq result value directly.
func decodeScalarValue(original *yaml.Node) any {
	var v any
	if err := original.Decode(&v); err != nil {
		return nil
	}
	return v
}

// valuesEqual reports whether a and b are the same value. Both sides
// are always json.Number for a number here -- Engine.Run normalizes
// gojq's result (see its own doc comment), and decodeScalarValue
// above decodes the same way convert.go's own YAML decoding does --
// but two json.Number values can still differ in string form for the
// same number (e.g. "3" vs "3.0"), so numbers are compared by value
// via asFloat rather than by exact string equality.
func valuesEqual(a, b any) bool {
	if an, aok := asFloat(a); aok {
		if bn, bok := asFloat(b); bok {
			return an == bn
		}
	}
	return reflect.DeepEqual(a, b)
}

// asFloat converts a json.Number to float64, for valuesEqual's
// by-value numeric comparison. It is not used for anything that needs
// exact precision, only for detecting "value did not change".
func asFloat(v any) (float64, bool) {
	n, ok := v.(json.Number)
	if !ok {
		return 0, false
	}
	f, err := n.Float64()
	return f, err == nil
}

// reconcileMapping reconciles a mapping. Keys result and original
// share (by name, not position) are reconciled recursively; a key
// result no longer has is omitted; a key only in result (added by the
// filter) has nothing to reuse, so it is rendered plainly. Under
// opts.SortKeys, every key (reused or new) is emitted in one
// alphabetical order; otherwise, reused keys keep original's own key
// order, with new keys rendered afterward in a fixed (sorted) order,
// since result's own map iteration order is not meaningful and Go's
// is not stable.
func reconcileMapping(original *yaml.Node, result map[string]any, opts FormatOptions) *yaml.Node {
	origKeyNode := map[string]*yaml.Node{}
	origValueNode := map[string]*yaml.Node{}
	origOrder := make([]string, 0, len(original.Content)/2)
	for _, pair := range mappingPairs(original) {
		origKeyNode[pair.Key.Value] = pair.Key
		origValueNode[pair.Key.Value] = pair.Value
		origOrder = append(origOrder, pair.Key.Value)
	}

	mapping := &yaml.Node{Kind: yaml.MappingNode, Style: original.Style}

	seen := make(map[string]bool, len(origOrder))
	reused := make([]string, 0, len(origOrder))
	for _, key := range origOrder {
		if _, ok := result[key]; !ok {
			continue // removed by the filter.
		}
		seen[key] = true
		reused = append(reused, key)
	}

	newKeys := make([]string, 0, len(result)-len(seen))
	for key := range result {
		if !seen[key] {
			newKeys = append(newKeys, key)
		}
	}
	sort.Strings(newKeys)

	appendReused := func(key string) {
		// Reuse the original key Node itself (not a fresh one), so a
		// comment attached to the key -- e.g. a head comment on the
		// first entry of a mapping -- survives even though the key's
		// *value* may have changed.
		mapping.Content = append(mapping.Content, origKeyNode[key], reconcile(origValueNode[key], result[key], opts))
	}
	appendNew := func(key string) {
		mapping.Content = append(mapping.Content, scalarKeyNode(key), yamlNodeFor(result[key], opts))
	}

	if opts.SortKeys {
		all := append(append([]string{}, reused...), newKeys...)
		sort.Strings(all)
		for _, key := range all {
			if seen[key] {
				appendReused(key)
			} else {
				appendNew(key)
			}
		}
	} else {
		for _, key := range reused {
			appendReused(key)
		}
		for _, key := range newKeys {
			appendNew(key)
		}
	}

	return mapping
}

// reconcileSequence reconciles a sequence by index: an index present
// in both is reconciled recursively; an index only in result (the
// sequence grew) is rendered plainly; an index only in original (the
// sequence shrank) is dropped.
func reconcileSequence(original *yaml.Node, result []any, opts FormatOptions) *yaml.Node {
	seq := &yaml.Node{Kind: yaml.SequenceNode, Style: original.Style}
	for i, v := range result {
		if i < len(original.Content) {
			seq.Content = append(seq.Content, reconcile(original.Content[i], v, opts))
		} else {
			seq.Content = append(seq.Content, yamlNodeFor(v, opts))
		}
	}
	return seq
}

// scalarKeyNode builds a plain string-typed Node for a mapping key.
func scalarKeyNode(key string) *yaml.Node {
	n := &yaml.Node{}
	n.SetString(key)
	return n
}
