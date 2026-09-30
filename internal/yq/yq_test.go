// Copyright 2026 Outreach Corporation. Licensed under the Apache License 2.0.

// Description: Tests for YAML decoding into gojq's plain value shape.

package yq

import (
	"encoding/json"
	"math"
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

// TestNormalizeYAML_LeadingDotFloat is a regression test for a real
// bug found during review: go-yaml's own generic decode-into-any
// mistypes some valid YAML 1.1 number literals as a plain Go float64
// instead of json.Number -- ".5" (no digit before the leading dot) and
// "-0" are the two found in practice, among values go-yaml otherwise
// classifies as numeric (node.Tag "!!float") -- silently, with no
// error. An uncorrected float64 here would always look "changed" to
// Engine.Run's normalizeValueCOW, forcing its containing object to be
// rebuilt and lose its recorded KeyOrder, reordering untouched keys
// under even a no-op identity filter.
func TestNormalizeYAML_LeadingDotFloat(t *testing.T) {
	v, _, err := NormalizeYAML([]byte("foo: .5\nbar: -0\nbaz: 1\nqux: -.5\n"))
	assert.NilError(t, err)

	m, ok := v.(map[string]any)
	assert.Assert(t, ok, "expected map[string]any, got %T", v)

	// Values must be valid JSON too, not just numerically correct:
	// format.go's JSON encoder writes a json.Number's text verbatim
	// with no validation of its own, and JSON (unlike YAML) requires a
	// digit before a decimal point, so ".5"/"-.5" must become
	// "0.5"/"-0.5", not keep their own source text.
	assert.Equal(t, m["foo"], json.Number("0.5"))
	assert.Equal(t, m["bar"], json.Number("-0"))
	assert.Equal(t, m["baz"], json.Number("1"))
	assert.Equal(t, m["qux"], json.Number("-0.5"))
}

// TestNormalizeYAML_AliasedLeadingDotFloat is a regression test for a
// real bug found during review: unlike a mapping/sequence (a reference
// type, so an alias to one shares its anchor's already-fixed-up
// identity), go-yaml decodes an aliased scalar independently -- so
// fixupNumberScalars following node.Alias, not just returning early
// for an AliasNode the way recordKeyOrder does for an unrelated
// reason, is what keeps an anchor's own fix from being lost at every
// place that aliases it.
func TestNormalizeYAML_AliasedLeadingDotFloat(t *testing.T) {
	v, _, err := NormalizeYAML([]byte("base: &n .5\nfoo: *n\n"))
	assert.NilError(t, err)
	m := v.(map[string]any)

	assert.Equal(t, m["base"], json.Number("0.5"))
	assert.Equal(t, m["foo"], json.Number("0.5"))
}

// TestNormalizeYAML_PreservesCorrectlyResolvedNumberForms confirms the
// fix for the leading-dot case doesn't discard go-yaml's own already-
// correct number resolution for other forms (hex, octal, underscore
// separators): these decode as json.Number holding plain decimal text
// (not the original source spelling), which TestNormalizeYAML_
// LeadingDotFloat's own fix must not override by reconstructing from
// raw source text unconditionally.
func TestNormalizeYAML_PreservesCorrectlyResolvedNumberForms(t *testing.T) {
	v, _, err := NormalizeYAML([]byte("hex: 0x1F\noctal: 010\nunderscore: 1_000\n"))
	assert.NilError(t, err)
	m := v.(map[string]any)

	assert.Equal(t, m["hex"], json.Number("31"))
	assert.Equal(t, m["octal"], json.Number("8"))
	assert.Equal(t, m["underscore"], json.Number("1000"))
}

// TestNormalizeYAML_InfinityStaysFloat64 confirms .inf/.nan are left
// as float64, deliberately: json.Number has no valid JSON text for
// either, so Engine.Run's normalizeValue rejecting a non-finite
// float64 with ErrNonFiniteNumber (already true for an arithmetic
// result) also applies to one appearing literally in the input,
// instead of this package inventing a second, different way to fail.
func TestNormalizeYAML_InfinityStaysFloat64(t *testing.T) {
	v, _, err := NormalizeYAML([]byte("a: .inf\n"))
	assert.NilError(t, err)
	m := v.(map[string]any)
	f, ok := m["a"].(float64)
	assert.Assert(t, ok, "expected float64, got %T", m["a"])
	assert.Assert(t, math.IsInf(f, 1))
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

	root := docs[0].(map[string]any)
	items := root["items"].([]any)
	assert.Equal(t, len(items), 2)

	first := items[0].(map[string]any)
	firstKeys, found := order.order(first)
	assert.Assert(t, found)
	assert.DeepEqual(t, firstKeys, []string{"z", "a"})

	second := items[1].(map[string]any)
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

	root := docs[0].(map[string]any)
	child := root["child"].(map[string]any)
	assert.Equal(t, len(child), 3) // a, b, c

	_, found := order.order(child)
	assert.Assert(t, !found, "merge-key mapping should not have a usable recorded order")
}
