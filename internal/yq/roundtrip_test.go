// Copyright 2026 Outreach Corporation. Licensed under the Apache License 2.0.

// Description: Tests for -Y/--yaml-roundtrip's path-based
// reconciliation.

package yq

import (
	"context"
	"testing"

	yaml "github.com/itchyny/go-yaml"
	"gotest.tools/v3/assert"
)

// decodeForRoundtrip decodes a single YAML document into both its
// plain gojq value and its comment/anchor/style-preserving root Node,
// as -Y's real callers do via SplitYAMLDocumentsWithNodes.
func decodeForRoundtrip(t *testing.T, doc string) (any, *yaml.Node) {
	t.Helper()
	values, nodes, _, err := SplitYAMLDocumentsWithNodes([]byte(doc))
	assert.NilError(t, err)
	assert.Equal(t, len(values), 1)
	return values[0], nodes[0]
}

// TestRoundtrip_Identity confirms a plain `-Y .` round-trip reproduces
// the input byte-for-byte, comments included.
func TestRoundtrip_Identity(t *testing.T) {
	doc := "# a head comment\nfoo: 1 # a line comment\nbar:\n  - a\n  - b\n"
	v, node := decodeForRoundtrip(t, doc)

	e, err := Compile(".", nil)
	assert.NilError(t, err)
	results, err := e.Run(context.Background(), v)
	assert.NilError(t, err)
	assert.Equal(t, len(results), 1)

	got, err := Roundtrip(node, results[0], FormatOptions{})
	assert.NilError(t, err)
	assert.Equal(t, string(got), "# a head comment\nfoo: 1 # a line comment\nbar:\n  - a\n  - b")
}

// TestRoundtrip_ScalarEdit confirms `-Y '.foo = "x"'` preserves the
// comment on foo while updating its value.
func TestRoundtrip_ScalarEdit(t *testing.T) {
	doc := "foo: 1 # keep me\nbar: 2\n"
	v, node := decodeForRoundtrip(t, doc)

	e, err := Compile(`.foo = "x"`, nil)
	assert.NilError(t, err)
	results, err := e.Run(context.Background(), v)
	assert.NilError(t, err)

	got, err := Roundtrip(node, results[0], FormatOptions{})
	assert.NilError(t, err)
	assert.Equal(t, string(got), "foo: x # keep me\nbar: 2")
}

// TestRoundtrip_KaiagatewayMerge exercises the real production case
// this feature exists for: an authored, comment-bearing YAML document
// merged with `. * {data: {...}}`. gojq's `*` deep-merges two values
// under the same key only when both are objects; here the original
// "data" is a scalar, so the merge replaces it outright with the new
// object, and its comment is correctly dropped -- while "kind", a key
// the merge never touches, keeps its comment.
func TestRoundtrip_KaiagatewayMerge(t *testing.T) {
	doc := "kind: ConfigMap # keep\ndata: old-value # dropped, replaced below\n"
	v, node := decodeForRoundtrip(t, doc)

	e, err := Compile(". * {data: {new: 1}}", nil)
	assert.NilError(t, err)
	results, err := e.Run(context.Background(), v)
	assert.NilError(t, err)

	got, err := Roundtrip(node, results[0], FormatOptions{})
	assert.NilError(t, err)
	assert.Equal(t, string(got), "kind: ConfigMap # keep\ndata:\n  new: 1")
}

// TestRoundtrip_KaiagatewayMerge_PreservesUntouchedNestedKeys confirms
// the recursive-merge case too: when "data" is itself an object on
// both sides, `*` deep-merges it (per real jq semantics), so an
// existing sub-key the merge doesn't mention keeps its comment, and
// only a newly-added sub-key has none.
func TestRoundtrip_KaiagatewayMerge_PreservesUntouchedNestedKeys(t *testing.T) {
	doc := "data:\n  old: value # keep\n"
	v, node := decodeForRoundtrip(t, doc)

	e, err := Compile(". * {data: {new: 1}}", nil)
	assert.NilError(t, err)
	results, err := e.Run(context.Background(), v)
	assert.NilError(t, err)

	got, err := Roundtrip(node, results[0], FormatOptions{})
	assert.NilError(t, err)
	assert.Equal(t, string(got), "data:\n  old: value # keep\n  new: 1")
}

// TestRoundtrip_AddedKey confirms a filter-added key (not present in
// the original) renders plainly, with no comment to preserve.
func TestRoundtrip_AddedKey(t *testing.T) {
	doc := "a: 1 # keep\n"
	v, node := decodeForRoundtrip(t, doc)

	e, err := Compile(". + {b: 2}", nil)
	assert.NilError(t, err)
	results, err := e.Run(context.Background(), v)
	assert.NilError(t, err)

	got, err := Roundtrip(node, results[0], FormatOptions{})
	assert.NilError(t, err)
	assert.Equal(t, string(got), "a: 1 # keep\nb: 2")
}

// TestRoundtrip_RemovedKey confirms a key the filter dropped is
// omitted from the output.
func TestRoundtrip_RemovedKey(t *testing.T) {
	doc := "a: 1\nb: 2 # drop me\n"
	v, node := decodeForRoundtrip(t, doc)

	e, err := Compile("del(.b)", nil)
	assert.NilError(t, err)
	results, err := e.Run(context.Background(), v)
	assert.NilError(t, err)

	got, err := Roundtrip(node, results[0], FormatOptions{})
	assert.NilError(t, err)
	assert.Equal(t, string(got), "a: 1")
}

// TestRoundtrip_SequenceEdit confirms an untouched sequence element
// keeps its comment, an edited element keeps its comment too (a
// scalar edit's comment always survives, per reconcileScalar -- the
// same behavior TestRoundtrip_ScalarEdit confirms for a mapping
// value), and a newly-appended element has no comment to preserve.
func TestRoundtrip_SequenceEdit(t *testing.T) {
	doc := "items:\n  - a # first\n  - b # second\n"
	v, node := decodeForRoundtrip(t, doc)

	e, err := Compile(`.items[1] = "c" | .items += ["d"]`, nil)
	assert.NilError(t, err)
	results, err := e.Run(context.Background(), v)
	assert.NilError(t, err)

	got, err := Roundtrip(node, results[0], FormatOptions{})
	assert.NilError(t, err)
	assert.Equal(t, string(got), "items:\n  - a # first\n  - c # second\n  - d")
}
