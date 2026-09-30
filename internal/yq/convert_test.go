// Copyright 2026 Outreach Corporation. Licensed under the Apache License 2.0.

// Description: Tests for YAML decoding into gojq's plain value shape.

package yq

import (
	"encoding/json"
	"testing"

	"gotest.tools/v3/assert"
)

// TestNormalizeYAML confirms NormalizeYAML decodes scalars, mappings,
// and sequences into the plain types gojq operates on -- in
// particular, that integers decode as json.Number (accepted by gojq)
// rather than float64, so an integer round-trip never becomes "3.0".
func TestNormalizeYAML(t *testing.T) {
	v, _, err := NormalizeYAML([]byte("arguments:\n  enableCgo: true\n  retries: 3\n  timeout: 1.5\n  name: null\n"))
	assert.NilError(t, err)

	m, ok := v.(map[string]any)
	assert.Assert(t, ok, "expected map[string]any, got %T", v)

	args, ok := m["arguments"].(map[string]any)
	assert.Assert(t, ok, "expected map[string]any, got %T", m["arguments"])

	assert.Equal(t, args["enableCgo"], true)
	assert.Equal(t, args["retries"], json.Number("3"))
	assert.Equal(t, args["timeout"], json.Number("1.5"))
	assert.Assert(t, args["name"] == nil)
}

// TestNormalizeYAML_RejectsMultiDocument confirms NormalizeYAML errors
// out on a multi-document stream rather than silently picking one.
func TestNormalizeYAML_RejectsMultiDocument(t *testing.T) {
	_, _, err := NormalizeYAML([]byte("a: 1\n---\nb: 2\n"))
	assert.ErrorContains(t, err, "expected exactly one YAML document, got 2")
}

// TestSplitYAMLDocuments confirms a `---`-separated stream decodes
// into one value per document, matching devconfig.sh's use over a
// multi-document Kubernetes manifest stream.
func TestSplitYAMLDocuments(t *testing.T) {
	docs, _, err := SplitYAMLDocuments([]byte("kind: ConfigMap\nname: a\n---\nkind: Secret\nname: b\n"))
	assert.NilError(t, err)
	assert.Equal(t, len(docs), 2)

	first, ok := docs[0].(map[string]any)
	assert.Assert(t, ok)
	assert.Equal(t, first["kind"], "ConfigMap")

	second, ok := docs[1].(map[string]any)
	assert.Assert(t, ok)
	assert.Equal(t, second["kind"], "Secret")
}

// TestSplitYAMLDocuments_Empty confirms an empty input decodes to zero
// documents rather than erroring.
func TestSplitYAMLDocuments_Empty(t *testing.T) {
	docs, order, err := SplitYAMLDocuments([]byte(""))
	assert.NilError(t, err)
	assert.Equal(t, len(docs), 0)
	assert.Equal(t, len(order), 0)
}

// TestSplitYAMLDocuments_KeyOrder confirms the KeyOrder table records
// each mapping's original key order (not alphabetical), for both the
// root object and a nested object -- the default-output case in
// "Default key order vs. -S".
func TestSplitYAMLDocuments_KeyOrder(t *testing.T) {
	docs, order, err := SplitYAMLDocuments([]byte("zeta: 1\nalpha:\n  gamma: 1\n  beta: 2\nbeta: 3\n"))
	assert.NilError(t, err)
	assert.Equal(t, len(docs), 1)

	root, ok := docs[0].(map[string]any)
	assert.Assert(t, ok)

	rootKeys, found := order.order(root)
	assert.Assert(t, found)
	assert.DeepEqual(t, rootKeys, []string{"zeta", "alpha", "beta"})

	nested, ok := root["alpha"].(map[string]any)
	assert.Assert(t, ok)

	nestedKeys, found := order.order(nested)
	assert.Assert(t, found)
	assert.DeepEqual(t, nestedKeys, []string{"gamma", "beta"})
}

// TestSplitYAMLDocuments_KeyOrder_SequenceOfMappings confirms key order
// is recorded for mappings nested inside a sequence too, e.g. a
// Kubernetes manifest's list of container specs.
func TestSplitYAMLDocuments_KeyOrder_SequenceOfMappings(t *testing.T) {
	docs, order, err := SplitYAMLDocuments([]byte("items:\n  - z: 1\n    a: 2\n  - b: 3\n    c: 4\n"))
	assert.NilError(t, err)

	root := docs[0].(map[string]any) //nolint:errcheck,forcetypeassert // Why: test fixture.
	items := root["items"].([]any)   //nolint:errcheck,forcetypeassert // Why: test fixture.
	assert.Equal(t, len(items), 2)

	first := items[0].(map[string]any) //nolint:errcheck,forcetypeassert // Why: test fixture.
	firstKeys, found := order.order(first)
	assert.Assert(t, found)
	assert.DeepEqual(t, firstKeys, []string{"z", "a"})

	second := items[1].(map[string]any) //nolint:errcheck,forcetypeassert // Why: test fixture.
	secondKeys, found := order.order(second)
	assert.Assert(t, found)
	assert.DeepEqual(t, secondKeys, []string{"b", "c"})
}

// TestSplitYAMLDocuments_MergeKey confirms a YAML merge key ("<<") does
// not corrupt the key-order table: it is not itself recorded as a key,
// and a length mismatch against the merged-in result safely falls back
// to "no recorded order" rather than emitting a bogus "<<" key.
func TestSplitYAMLDocuments_MergeKey(t *testing.T) {
	docs, order, err := SplitYAMLDocuments([]byte("base: &base\n  a: 1\n  b: 2\nchild:\n  <<: *base\n  c: 3\n"))
	assert.NilError(t, err)

	root := docs[0].(map[string]any)        //nolint:errcheck,forcetypeassert // Why: test fixture.
	child := root["child"].(map[string]any) //nolint:errcheck,forcetypeassert // Why: test fixture.
	assert.Equal(t, len(child), 3)          // a, b, c

	_, found := order.order(child)
	assert.Assert(t, !found, "merge-key mapping should not have a usable recorded order")
}
