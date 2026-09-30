// Copyright 2026 Outreach Corporation. Licensed under the Apache License 2.0.

// Description: Tests for compiling and running jq filters against
// decoded YAML values.

package yq

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"gotest.tools/v3/assert"
)

// TestEngine_RealCallSites exercises filters taken directly from real
// devbase shell call sites, so the shell-to-Go migration is verifiable
// before shell/yq.sh is swapped over.
func TestEngine_RealCallSites(t *testing.T) {
	cases := []struct {
		name   string
		yaml   string
		filter string
		want   []any
	}{
		{
			// cgo-enabled.sh
			name:   "arguments.enableCgo",
			yaml:   "arguments:\n  enableCgo: true\n",
			filter: ".arguments.enableCgo",
			want:   []any{true},
		},
		{
			// bootstrap.sh
			name:   "select module by name",
			yaml:   "modules:\n  - name: a\n    version: v1\n  - name: b\n    version: v2\n",
			filter: `.modules[] | select(.name == "b") | .version`,
			want:   []any{"v2"},
		},
		{
			name:   "bracket key access",
			yaml:   "replacements:\n  github.com/getoutreach/devbase: /path\n",
			filter: `.replacements["github.com/getoutreach/devbase"]`,
			want:   []any{"/path"},
		},
		{
			name:   "identity on a mapping",
			yaml:   "a: 1\nb: 2\n",
			filter: ".",
			want:   []any{map[string]any{"a": json.Number("1"), "b": json.Number("2")}},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			v, _, err := NormalizeYAML([]byte(c.yaml))
			assert.NilError(t, err)

			e, err := Compile(c.filter, nil)
			assert.NilError(t, err)

			got, err := e.Run(context.Background(), v)
			assert.NilError(t, err)
			assert.DeepEqual(t, got, c.want)
		})
	}
}

// TestEngine_IntegerRoundTrip confirms an integer passed through a
// filter survives as json.Number rather than becoming a float (i.e.
// never renders as "3.0").
func TestEngine_IntegerRoundTrip(t *testing.T) {
	v, _, err := NormalizeYAML([]byte("count: 3\n"))
	assert.NilError(t, err)

	e, err := Compile(".count", nil)
	assert.NilError(t, err)

	got, err := e.Run(context.Background(), v)
	assert.NilError(t, err)
	assert.Equal(t, len(got), 1)
	assert.Equal(t, got[0], json.Number("3"))
}

// TestEngine_NonFiniteNumber confirms a NaN or infinite result is
// rejected rather than silently rendered as an invalid JSON/YAML
// token, both as a top-level scalar and nested inside an object, and
// that an earlier, finite result in the same filter is still returned
// alongside the error.
func TestEngine_NonFiniteNumber(t *testing.T) {
	v, _, err := NormalizeYAML([]byte("a: 1\n"))
	assert.NilError(t, err)

	e, err := Compile(`1, (-1|sqrt)`, nil)
	assert.NilError(t, err)

	got, err := e.Run(context.Background(), v)
	assert.Assert(t, errors.Is(err, ErrNonFiniteNumber))
	assert.DeepEqual(t, got, []any{json.Number("1")})

	e, err = Compile(`{x: (-1|sqrt)}`, nil)
	assert.NilError(t, err)

	_, err = e.Run(context.Background(), v)
	assert.Assert(t, errors.Is(err, ErrNonFiniteNumber))
}

// TestEngine_RunDoesNotMutateSharedVars is a regression test for a real
// bug found during review: normalizeValue used to mutate a filter
// result's maps/slices in place, which corrupted a --argjson binding
// when a filter returned it verbatim, since that same shared map is
// reused, unchanged, by every Run call on the same *Engine (including,
// a later caller found, concurrently).
func TestEngine_RunDoesNotMutateSharedVars(t *testing.T) {
	extra := map[string]any{"n": 1}
	vars := map[string]any{"extra": extra}

	e, err := Compile("$extra", vars)
	assert.NilError(t, err)

	got, err := e.Run(context.Background(), nil)
	assert.NilError(t, err)
	assert.DeepEqual(t, got, []any{map[string]any{"n": json.Number("1")}})

	// extra, the original map still referenced by vars, must be
	// untouched -- its "n" is still a plain int, not json.Number.
	assert.DeepEqual(t, extra, map[string]any{"n": 1})
}

// TestEngine_RunPreservesUnchangedMapIdentity is a regression test for
// a real bug an earlier, naive fix for the race
// TestEngine_RunDoesNotMutateSharedVars covers introduced: always
// returning a freshly built map/slice (rather than only when something
// inside actually needed converting) broke format.go's/roundtrip.go's
// KeyOrder, which looks up a sub-object's recorded key order by its
// map's own pointer -- a sub-object already fully json.Number'd (true
// of anything SplitYAMLDocuments decoded, on any path a filter didn't
// touch) must come back from Run with that same pointer, or the lookup
// silently falls back to alphabetical order for it.
func TestEngine_RunPreservesUnchangedMapIdentity(t *testing.T) {
	untouched := map[string]any{"z": json.Number("1"), "a": json.Number("2")}
	input := map[string]any{"untouched": untouched, "n": json.Number("1")}

	e, err := Compile(".", nil)
	assert.NilError(t, err)

	got, err := e.Run(context.Background(), input)
	assert.NilError(t, err)
	assert.Equal(t, len(got), 1)

	gotMap, ok := got[0].(map[string]any)
	assert.Assert(t, ok)
	gotUntouched, ok := gotMap["untouched"].(map[string]any)
	assert.Assert(t, ok)

	assert.Equal(t, reflect.ValueOf(gotUntouched).Pointer(), reflect.ValueOf(untouched).Pointer())
}

// TestEngine_RunPreservesUnchangedSliceIdentity mirrors
// TestEngine_RunPreservesUnchangedMapIdentity for the []any case of the
// same copy-on-write logic: the filter forces the outer map to be
// rebuilt (by assigning a plain int, which needs converting), but
// "untouched" itself is never touched by the filter, so it must come
// back as the exact same slice.
func TestEngine_RunPreservesUnchangedSliceIdentity(t *testing.T) {
	untouched := []any{json.Number("1"), json.Number("2")}
	input := map[string]any{"untouched": untouched, "n": json.Number("1")}

	e, err := Compile(".n = 1", nil)
	assert.NilError(t, err)

	got, err := e.Run(context.Background(), input)
	assert.NilError(t, err)
	assert.Equal(t, len(got), 1)

	gotMap, ok := got[0].(map[string]any)
	assert.Assert(t, ok)
	gotUntouched, ok := gotMap["untouched"].([]any)
	assert.Assert(t, ok)

	assert.Equal(t, reflect.ValueOf(gotUntouched).Pointer(), reflect.ValueOf(untouched).Pointer())
}

// TestBuildVars_Arg confirms --arg binds a raw string variable.
func TestBuildVars_Arg(t *testing.T) {
	vars, err := BuildVars(&yqFlags{Arg: map[string]string{"name": "value"}})
	assert.NilError(t, err)
	assert.DeepEqual(t, vars, map[string]any{"name": "value"})
}

// TestBuildVars_ArgJSON confirms --argjson strictly parses its value
// and rejects malformed JSON with a clear error, rather than a lenient
// YAML parse.
func TestBuildVars_ArgJSON(t *testing.T) {
	vars, err := BuildVars(&yqFlags{ArgJSON: map[string]string{"n": "3"}})
	assert.NilError(t, err)
	assert.DeepEqual(t, vars, map[string]any{"n": json.Number("3")})

	_, err = BuildVars(&yqFlags{ArgJSON: map[string]string{"bad": "not json"}})
	assert.ErrorContains(t, err, "--argjson bad")
}

// TestBuildVars_SlurpFile confirms --slurpfile binds the array of all
// documents in the named file, matching box.sh:52-53's
// `--slurpfile boxconf <file> '. * {config: $boxconf[0]}'` usage.
func TestBuildVars_SlurpFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "box.yaml")
	assert.NilError(t, os.WriteFile(path, []byte("org: outreach\n"), 0o600))

	vars, err := BuildVars(&yqFlags{SlurpFile: map[string]string{"boxconf": path}})
	assert.NilError(t, err)

	docs, ok := vars["boxconf"].([]any)
	assert.Assert(t, ok)
	assert.Equal(t, len(docs), 1)

	doc, ok := docs[0].(map[string]any)
	assert.Assert(t, ok)
	assert.Equal(t, doc["org"], "outreach")

	// Exercise the actual merge filter from box.sh.
	v, _, err := NormalizeYAML([]byte("name: myservice\n"))
	assert.NilError(t, err)

	e, err := Compile(". * {config: $boxconf[0]}", vars)
	assert.NilError(t, err)

	got, err := e.Run(context.Background(), v)
	assert.NilError(t, err)
	assert.Equal(t, len(got), 1)

	merged, ok := got[0].(map[string]any)
	assert.Assert(t, ok)
	assert.Equal(t, merged["name"], "myservice")

	config, ok := merged["config"].(map[string]any)
	assert.Assert(t, ok)
	assert.Equal(t, config["org"], "outreach")
}

// TestEngine_MultiDocumentSelect confirms select(.kind == "ConfigMap")
// over a multi-document YAML stream matches devconfig.sh's usage.
func TestEngine_MultiDocumentSelect(t *testing.T) {
	docs, _, err := SplitYAMLDocuments([]byte("kind: ConfigMap\nname: a\n---\nkind: Secret\nname: b\n"))
	assert.NilError(t, err)

	e, err := Compile(`select(.kind == "ConfigMap")`, nil)
	assert.NilError(t, err)

	matched := make([]any, 0, len(docs))
	for _, doc := range docs {
		got, err := e.Run(context.Background(), doc)
		assert.NilError(t, err)
		matched = append(matched, got...)
	}
	assert.Equal(t, len(matched), 1)
	m, ok := matched[0].(map[string]any)
	assert.Assert(t, ok)
	assert.Equal(t, m["name"], "a")
}
