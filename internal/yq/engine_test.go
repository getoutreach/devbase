// Copyright 2026 Outreach Corporation. Licensed under the Apache License 2.0.

// Description: Tests for compiling and running jq filters against
// decoded YAML values.

package yq

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
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
