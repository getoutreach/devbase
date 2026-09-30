// Copyright 2026 Outreach Corporation. Licensed under the Apache License 2.0.

// Description: Tests for rendering gojq result values as JSON or YAML.

package yq

import (
	"context"
	"testing"

	"gotest.tools/v3/assert"
)

// TestFormatValue_RawOutput confirms -r unwraps a top-level string but
// leaves a non-string result formatted, matching box.sh's `-r .` on a
// mapping.
func TestFormatValue_RawOutput(t *testing.T) {
	got, err := FormatValue("hello", FormatOptions{Raw: true})
	assert.NilError(t, err)
	assert.Equal(t, string(got), "hello")

	got, err = FormatValue(map[string]any{"a": "1"}, FormatOptions{Raw: true})
	assert.NilError(t, err)
	assert.Equal(t, string(got), "{\n  \"a\": \"1\"\n}")
}

// TestFormatValue_CompactOutput confirms -c disables pretty-printing.
func TestFormatValue_CompactOutput(t *testing.T) {
	got, err := FormatValue(map[string]any{"a": "1", "b": "2"}, FormatOptions{Compact: true})
	assert.NilError(t, err)
	assert.Equal(t, string(got), `{"a":"1","b":"2"}`)
}

// TestFormatValue_DefaultKeyOrder confirms an object with a recorded
// key order is emitted in that order by default and alphabetically
// under -S -- the "Default key order vs. -S" design.
func TestFormatValue_DefaultKeyOrder(t *testing.T) {
	docs, order, err := SplitYAMLDocuments([]byte("zeta: 1\nalpha: 2\nbeta: 3\n"))
	assert.NilError(t, err)
	v := docs[0]

	got, err := FormatValue(v, FormatOptions{Compact: true, Order: order})
	assert.NilError(t, err)
	assert.Equal(t, string(got), `{"zeta":1,"alpha":2,"beta":3}`)

	got, err = FormatValue(v, FormatOptions{Compact: true, Order: order, SortKeys: true})
	assert.NilError(t, err)
	assert.Equal(t, string(got), `{"alpha":2,"beta":3,"zeta":1}`)
}

// TestFormatValue_KeyOrder_MutationSplit confirms the split confirmed
// in "What was verified": a filter that mutates a path (.arguments.
// enableCgo = false) loses recorded order on the root and on the
// mutated path's parent, while a sibling object not on that path keeps
// its recorded order.
func TestFormatValue_KeyOrder_MutationSplit(t *testing.T) {
	v, order, err := NormalizeYAML([]byte("zeta: 1\narguments:\n  enableCgo: true\n  retries: 3\nalpha:\n  z: 1\n  a: 2\n"))
	assert.NilError(t, err)

	e, err := Compile(".arguments.enableCgo = false", nil)
	assert.NilError(t, err)

	results, err := e.Run(context.Background(), v)
	assert.NilError(t, err)
	assert.Equal(t, len(results), 1)

	got, err := FormatValue(results[0], FormatOptions{Compact: true, Order: order})
	assert.NilError(t, err)

	// The root and .arguments (on the modified path) fall back to
	// alphabetical; .alpha (not on the modified path) keeps its
	// original order.
	assert.Equal(t, string(got), `{"alpha":{"z":1,"a":2},"arguments":{"enableCgo":false,"retries":3},"zeta":1}`)
}

// TestFormatValue_ASCIIOutput confirms -a escapes non-ASCII characters
// in JSON string output.
func TestFormatValue_ASCIIOutput(t *testing.T) {
	got, err := FormatValue("café", FormatOptions{ASCII: true})
	assert.NilError(t, err)
	assert.Equal(t, string(got), "\"caf\\u00e9\"")

	got, err = FormatValue("café", FormatOptions{})
	assert.NilError(t, err)
	assert.Equal(t, string(got), `"café"`)
}

// TestFormatValue_YAMLOutput confirms -y renders YAML instead of JSON,
// preserving default key order the same way JSON output does.
func TestFormatValue_YAMLOutput(t *testing.T) {
	docs, order, err := SplitYAMLDocuments([]byte("zeta: 1\nalpha: 2\n"))
	assert.NilError(t, err)

	got, err := FormatValue(docs[0], FormatOptions{YAML: true, Order: order})
	assert.NilError(t, err)
	assert.Equal(t, string(got), "zeta: 1\nalpha: 2")
}

// TestFormatValue_YAMLIndentlessLists confirms --indentless-lists
// produces compact sequence indentation via
// github.com/itchyny/go-yaml's Encoder.CompactSeqIndent: the "-"
// aligns with a nested mapping key (2 spaces) instead of getting its
// own extra indent level beyond that (4 spaces, the default).
func TestFormatValue_YAMLIndentlessLists(t *testing.T) {
	v := map[string]any{"list": []any{"a", "b"}}

	indented, err := FormatValue(v, FormatOptions{YAML: true})
	assert.NilError(t, err)
	assert.Equal(t, string(indented), "list:\n  - a\n  - b")

	compact, err := FormatValue(v, FormatOptions{YAML: true, Indentless: true})
	assert.NilError(t, err)
	assert.Equal(t, string(compact), "list:\n- a\n- b")
}

// TestFormatValue_IntegerRoundTrip confirms a decoded integer renders
// as "3", never "3.0", in both JSON and YAML output.
func TestFormatValue_IntegerRoundTrip(t *testing.T) {
	v, _, err := NormalizeYAML([]byte("count: 3\n"))
	assert.NilError(t, err)

	json, err := FormatValue(v, FormatOptions{Compact: true})
	assert.NilError(t, err)
	assert.Equal(t, string(json), `{"count":3}`)

	yamlOut, err := FormatValue(v, FormatOptions{YAML: true})
	assert.NilError(t, err)
	assert.Equal(t, string(yamlOut), "count: 3")
}
