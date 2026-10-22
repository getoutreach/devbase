// Copyright 2026 Outreach Corporation. Licensed under the Apache License 2.0.

// Description: YAML decoding into the plain values gojq operates on.

// Package yq implements devbase's `yq` subcommand: a jq-compatible
// query engine over YAML input, built directly on gojq's compiler and
// runtime (github.com/itchyny/gojq) rather than shelling out to a gojq
// or python-yq binary.
package yq

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"reflect"
	"strings"

	yaml "github.com/itchyny/go-yaml"
)

// ErrNotSingleDocument is returned by NormalizeYAML when its input
// contains anything other than exactly one YAML document.
var ErrNotSingleDocument = errors.New("expected exactly one YAML document")

// errMalformedDocumentNode signals an invariant violated by the
// decoder itself (every DocumentNode it produces has exactly one
// child), not a user-facing input error.
var errMalformedDocumentNode = errors.New("yaml document node must have exactly one child")

// KeyOrder records, for a decoded map[string]any, the order its keys
// appeared in the source document, keyed by the map's own identity
// (its runtime pointer). Encoding consults this table to reproduce
// insertion order by default and falls back to alphabetical order when
// a map is missing from it or its key count no longer matches -- which
// happens for any object a filter built or modified, since gojq never
// mutates a decoded object in place (see format.go).
type KeyOrder map[uintptr][]string

// order returns the recorded key order for m, and whether one was
// found with a matching key count.
func (o KeyOrder) order(m map[string]any) ([]string, bool) {
	keys, ok := o[reflect.ValueOf(m).Pointer()]
	if !ok || len(keys) != len(m) {
		return nil, false
	}
	return keys, true
}

// NormalizeYAML decodes the single YAML document in b into the plain
// value shape gojq expects (nil, bool, json.Number, string, []any,
// map[string]any), along with the KeyOrder table for every object
// decoded along the way. It is an error for b to contain more than one
// document; use SplitYAMLDocuments for a multi-document stream.
func NormalizeYAML(b []byte) (any, KeyOrder, error) {
	docs, order, err := SplitYAMLDocuments(b)
	if err != nil {
		return nil, nil, err
	}
	if len(docs) != 1 {
		return nil, nil, fmt.Errorf("%w, got %d", ErrNotSingleDocument, len(docs))
	}
	return docs[0], order, nil
}

// SplitYAMLDocuments decodes every `---`-separated YAML document in b
// into the plain value shape gojq expects, along with a KeyOrder table
// shared across all of them.
func SplitYAMLDocuments(b []byte) ([]any, KeyOrder, error) {
	values, _, order, err := SplitYAMLDocumentsWithNodes(b)
	return values, order, err
}

// SplitYAMLDocumentsWithNodes behaves like SplitYAMLDocuments, but
// also returns each document's root Node (unwrapped from its
// DocumentNode wrapper), comments/anchors/style intact, for -Y's
// path-based reconciliation (see roundtrip.go). nodes[i] corresponds
// to values[i].
func SplitYAMLDocumentsWithNodes(b []byte) (values []any, nodes []*yaml.Node, order KeyOrder, err error) {
	dec := yaml.NewDecoder(bytes.NewReader(b))
	order = KeyOrder{}

	for {
		var node yaml.Node
		if err := dec.Decode(&node); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, nil, nil, fmt.Errorf("decode yaml document %d: %w", len(values)+1, err)
		}

		v, err := decodeNode(&node)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("decode yaml document %d: %w", len(values)+1, err)
		}
		if err := recordKeyOrder(&node, v, order); err != nil {
			return nil, nil, nil, fmt.Errorf("record key order for yaml document %d: %w", len(values)+1, err)
		}

		values = append(values, v)
		nodes = append(nodes, documentRoot(&node))
	}

	return values, nodes, order, nil
}

// documentRoot returns node's single child when node is a
// DocumentNode (as every document Decoder.Decode produces is), or
// node itself otherwise.
func documentRoot(node *yaml.Node) *yaml.Node {
	if node.Kind == yaml.DocumentNode && len(node.Content) == 1 {
		return node.Content[0]
	}
	return node
}

// decodeNode decodes node into gojq's plain value shape using the
// library's own decode logic for anchors, aliases, merge keys, maps,
// and slices, without re-implementing any of that here -- except for
// numeric scalars, where fixupNumberScalars corrects a real gap in the
// library's own scalar type resolution (see its own doc comment).
func decodeNode(node *yaml.Node) (any, error) {
	var v any
	if err := node.Decode(&v); err != nil {
		return nil, err
	}
	return fixupNumberScalars(node, v), nil
}

// fixupNumberScalars walks node and its already-decoded value v
// together, replacing a !!int/!!float-tagged scalar's value with a
// json.Number built directly from the node's own raw text (via
// jsonNumberText), rather than trusting whatever Go type go-yaml's
// generic decode-into-any happened to produce for it. node.Tag is
// resolved correctly even when the decode step isn't: "0", "-1",
// "0.5", "1.0", and "1e10" all decode to json.Number already, but "-0"
// and a leading-dot float like ".5" (both valid YAML 1.1 number
// syntax, both tagged !!float) decode to a plain Go float64 instead --
// silently, with no error. Left uncorrected, such a value reaching
// Engine.Run would always look "changed" to normalizeValueCOW (which
// must convert any float64/int it sees into json.Number), forcing its
// containing object to be rebuilt and lose its recorded KeyOrder -- so
// a mapping containing a bare ".5" would reorder its keys under even a
// no-op identity filter. Reconstructing from node.Value regardless of
// the decoded type (not just the cases go-yaml gets wrong) also
// upgrades a value go-yaml already decodes as json.Number but with
// non-JSON text, like "-.5": format.go's JSON encoder writes a
// json.Number's text verbatim with no validation of its own, and JSON
// (unlike YAML) requires a digit before a decimal point.
//
// An infinite or NaN float64 (from ".inf"/"-.inf"/".nan") is left
// alone, deliberately: json.Number has no valid JSON text for either,
// so wrapping one would just move the problem instead of fixing it.
// Engine.Run's normalizeValue already rejects a non-finite float64
// with ErrNonFiniteNumber when a filter's arithmetic produces one;
// leaving these as float64 here makes a literal ".inf" in the input
// hit that same, already-correct rejection instead of a new path.
func fixupNumberScalars(node *yaml.Node, v any) any {
	switch node.Kind {
	case yaml.DocumentNode:
		if len(node.Content) != 1 {
			// Malformed; decodeNode's own node.Decode(&v) call already
			// errors out on this before ever reaching here in
			// practice. Returning v unchanged (not recursing on node
			// itself, which would loop forever) is purely defensive.
			return v
		}
		return fixupNumberScalars(node.Content[0], v)

	case yaml.ScalarNode:
		if node.Tag != "!!int" && node.Tag != "!!float" {
			return v
		}
		if f, ok := v.(float64); ok && (math.IsInf(f, 0) || math.IsNaN(f)) {
			// .inf/-.inf/.nan: leave alone (see this function's own
			// doc comment on why).
			return v
		}
		if n, ok := v.(json.Number); ok && !isLeadingDotNumber(string(n)) {
			// Already a valid json.Number (go-yaml's own decode
			// handled this one correctly, including normalizing
			// hex/octal/underscore forms to plain decimal text) -- keep
			// it rather than rebuilding from node.Value, which for
			// those forms is the original, non-decimal text.
			return v
		}
		return json.Number(jsonNumberText(node.Value))

	case yaml.MappingNode:
		m, ok := v.(map[string]any)
		if !ok {
			return v
		}
		for _, pair := range mappingPairs(node) {
			m[pair.Key.Value] = fixupNumberScalars(pair.Value, m[pair.Key.Value])
		}
		return v

	case yaml.SequenceNode:
		arr, ok := v.([]any)
		if !ok {
			return v
		}
		for i, child := range node.Content {
			if i >= len(arr) {
				break
			}
			arr[i] = fixupNumberScalars(child, arr[i])
		}
		return v

	case yaml.AliasNode:
		// Unlike a mapping/sequence (a reference type, so an alias to
		// one shares its anchor's already-fixed-up identity), go-yaml
		// decodes an aliased scalar independently: node.Alias (the
		// referenced node, with its own real Tag/Value) must be
		// recursed into, or an aliased ".5"/"-0" would come out fixed
		// at its anchor but still broken at its alias.
		if node.Alias != nil {
			return fixupNumberScalars(node.Alias, v)
		}
		return v
	}
	return v
}

// jsonNumberText rewrites a !!int/!!float-tagged scalar's raw YAML
// text into valid JSON number syntax when it isn't already, without
// changing its numeric value: YAML 1.1 permits a leading-dot float
// with no digit before the point (".5", "-.5"), which JSON does not.
// Every other numeric form this package's number-resolution encounters
// (plain integers, "-0", "0.5", "1.0", exponents, and the hex/octal/
// underscore forms go-yaml itself already normalizes to plain decimal
// text before this ever runs) is already valid JSON text as-is.
func jsonNumberText(s string) string {
	switch {
	case strings.HasPrefix(s, "-."):
		return "-0" + s[1:]
	case strings.HasPrefix(s, "."):
		return "0" + s
	default:
		return s
	}
}

// isLeadingDotNumber reports whether s is a numeric literal missing
// the leading digit JSON requires before a decimal point (e.g. ".5",
// "-.5") -- the one way this package's own number handling can end up
// with a json.Number holding text that isn't valid JSON.
func isLeadingDotNumber(s string) bool {
	return strings.HasPrefix(s, ".") || strings.HasPrefix(s, "-.")
}

// recordKeyOrder walks node and its already-decoded value v together,
// recording the key order of every map[string]any decoded from a
// MappingNode into order, keyed by that map's identity. It assumes v
// was produced by decodeNode(node), so their shapes match exactly.
func recordKeyOrder(node *yaml.Node, v any, order KeyOrder) error {
	switch node.Kind {
	case yaml.DocumentNode:
		if len(node.Content) != 1 {
			return fmt.Errorf("%w: got %d", errMalformedDocumentNode, len(node.Content))
		}
		return recordKeyOrder(node.Content[0], v, order)

	case yaml.AliasNode:
		// The alias's own content was already recorded (or will be)
		// where the anchor itself was decoded; nothing extra to record
		// at the alias occurrence, since it decodes to the same value.
		return nil

	case yaml.MappingNode:
		m, ok := v.(map[string]any)
		if !ok {
			// Decoded into a non-map Go type (e.g. via a merge producing
			// something unexpected); nothing to record.
			return nil
		}

		pairs := mappingPairs(node)
		keys := make([]string, 0, len(pairs))
		for _, pair := range pairs {
			keys = append(keys, pair.Key.Value)
			if pair.Value.Kind == yaml.MappingNode || pair.Value.Kind == yaml.SequenceNode {
				if err := recordKeyOrder(pair.Value, m[pair.Key.Value], order); err != nil {
					return err
				}
			}
		}
		order[reflect.ValueOf(m).Pointer()] = keys

	case yaml.SequenceNode:
		arr, ok := v.([]any)
		if !ok {
			return nil
		}
		for i, child := range node.Content {
			if i >= len(arr) {
				break
			}
			if child.Kind == yaml.MappingNode || child.Kind == yaml.SequenceNode {
				if err := recordKeyOrder(child, arr[i], order); err != nil {
					return err
				}
			}
		}

	case yaml.ScalarNode:
		// No children to record key order for.
	}

	return nil
}

// mappingPair is one key/value Node pair of a MappingNode.
type mappingPair struct {
	Key, Value *yaml.Node
}

// mappingPairs returns node's real key/value pairs, in file order,
// skipping any "<<" merge key: it is not a real entry in the decoded
// map, and its own value node (an anchor, or a sequence of anchors)
// does not correspond to any single key's value, so including it
// would record a bogus key (recordKeyOrder) or try to reconcile
// against the wrong shape (roundtrip.go's reconcileMapping). This
// leaves the merged-in keys' own order/comments unreconciled -- an
// accepted, best-effort limitation, not a crash.
func mappingPairs(node *yaml.Node) []mappingPair {
	pairs := make([]mappingPair, 0, len(node.Content)/2)
	for i := 0; i+1 < len(node.Content); i += 2 {
		key := node.Content[i]
		if key.Value == "<<" && key.Tag == "!!merge" {
			continue
		}
		pairs = append(pairs, mappingPair{Key: key, Value: node.Content[i+1]})
	}
	return pairs
}
