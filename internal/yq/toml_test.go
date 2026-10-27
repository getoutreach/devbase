// Copyright 2026 Outreach Corporation. Licensed under the Apache License 2.0.

// Description: Tests for TOML decoding, key order, and tomlq flag validation.

package yq

import (
	"encoding/json"
	"testing"
	"time"

	"gotest.tools/v3/assert"
)

// TestDecodeTOML confirms decodeTOML produces the plain types gojq
// operates on: json.Number for numbers (never float64 or int64), and
// strings for every date and time form.
func TestDecodeTOML(t *testing.T) {
	v, _, err := decodeTOML([]byte(`
name = "svc"
retries = 3
ratio = 1.5
on = true
offset = 1979-05-27T07:32:00Z
local = 1979-05-27T07:32:00
day = 1979-05-27
clock = 07:32:00

[tools]
kubectl = "1.34.7"

[[lock]]
version = "1.0.0"
`))
	assert.NilError(t, err)

	m, ok := v.(map[string]any)
	assert.Assert(t, ok, "expected map[string]any, got %T", v)

	assert.Equal(t, m["name"], "svc")
	assert.Equal(t, m["retries"], json.Number("3"))
	assert.Equal(t, m["ratio"], json.Number("1.5"))
	assert.Equal(t, m["on"], true)
	assert.Equal(t, m["offset"], "1979-05-27T07:32:00Z")
	assert.Equal(t, m["local"], "1979-05-27T07:32:00")
	assert.Equal(t, m["day"], "1979-05-27")
	assert.Equal(t, m["clock"], "07:32:00")

	tools, ok := m["tools"].(map[string]any)
	assert.Assert(t, ok, "expected map[string]any, got %T", m["tools"])
	assert.Equal(t, tools["kubectl"], "1.34.7")

	lock, ok := m["lock"].([]any)
	assert.Assert(t, ok, "expected []any, got %T", m["lock"])
	assert.DeepEqual(t, lock, []any{map[string]any{"version": "1.0.0"}})
}

// TestDecodeTOML_DateTimeForms confirms each date and time form decodes
// to a string. It also guards the location names BurntSushi/toml gives
// zoneless values, which formatTOMLTime relies on.
func TestDecodeTOML_DateTimeForms(t *testing.T) {
	v, _, err := decodeTOML([]byte(`
offset = 1979-05-27T07:32:00.5-07:00
local = 1979-05-27T07:32:00.25
day = 1979-05-27
clock = 07:32:00
`))
	assert.NilError(t, err)
	assert.DeepEqual(t, v, map[string]any{
		"offset": "1979-05-27T07:32:00.5-07:00",
		"local":  "1979-05-27T07:32:00.25",
		"day":    "1979-05-27",
		"clock":  "07:32:00",
	})
}

// TestFormatTOMLTime_Zone confirms a time with a real zone is not
// mistaken for a zoneless one.
func TestFormatTOMLTime_Zone(t *testing.T) {
	assert.Equal(t, formatTOMLTime(time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC)), "2020-01-02T03:04:05Z")
}

// TestDecodeTOML_Empty confirms an empty document is an empty table,
// not null.
func TestDecodeTOML_Empty(t *testing.T) {
	v, _, err := decodeTOML(nil)
	assert.NilError(t, err)
	assert.DeepEqual(t, v, map[string]any{})
}

// TestDecodeTOML_Invalid confirms a syntax error is reported rather
// than yielding a partial document.
func TestDecodeTOML_Invalid(t *testing.T) {
	_, _, err := decodeTOML([]byte("a = \n"))
	assert.ErrorContains(t, err, "decode toml")
}

// TestDecodeTOML_NonFiniteNumber confirms a TOML inf is not wrapped as
// a json.Number holding invalid JSON text, so Engine.Run rejects it.
func TestDecodeTOML_NonFiniteNumber(t *testing.T) {
	v, _, err := decodeTOML([]byte("x = inf\n"))
	assert.NilError(t, err)

	engine, err := Compile(".", nil)
	assert.NilError(t, err)
	_, err = engine.Run(t.Context(), v)
	assert.ErrorIs(t, err, ErrNonFiniteNumber)
}

// TestValidateTOML confirms each flag with no meaning for TOML input is
// rejected, and that ordinary jq flags are not.
func TestValidateTOML(t *testing.T) {
	for _, tt := range []struct {
		name string
		args []string
	}{
		{"-y", []string{"-y", "."}},
		{"-Y", []string{"-Y", "."}},
		{"--indentless-lists", []string{"--indentless-lists", "."}},
		{"-i", []string{"-i", "."}},
		{"-t", []string{"-t", "."}},
		{"-T", []string{"-T", "."}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			flags, _, err := parseYqFlags(tt.args)
			assert.NilError(t, err)
			assert.ErrorIs(t, validateTOML(flags), errTOMLUnsupportedFlag)
		})
	}

	flags, _, err := parseYqFlags([]string{"-r", "-c", "-S", "."})
	assert.NilError(t, err)
	assert.NilError(t, validateTOML(flags))
}

// TestDecodeTOML_DuplicateKey confirms a key defined twice is an error,
// including an array key, which BurntSushi/toml accepted before v1.6.0.
func TestDecodeTOML_DuplicateKey(t *testing.T) {
	for _, doc := range []string{"a = 1\na = 2\n", "arr = [1]\narr = [2]\n"} {
		_, _, err := decodeTOML([]byte(doc))
		assert.ErrorContains(t, err, "already been defined")
	}
}

// tomlKeyOrder returns the key order decodeTOML recorded for the map at
// path in doc, failing the test if there is none.
func tomlKeyOrder(t *testing.T, doc string, path ...any) []string {
	t.Helper()

	v, order, err := decodeTOML([]byte(doc))
	assert.NilError(t, err)
	for _, step := range path {
		switch s := step.(type) {
		case string:
			v = v.(map[string]any)[s]
		case int:
			v = v.([]any)[s]
		}
	}
	m, ok := v.(map[string]any)
	assert.Assert(t, ok, "expected map[string]any, got %T", v)
	keys, ok := order.order(m)
	assert.Assert(t, ok, "no recorded key order for %v", path)
	return keys
}

// TestDecodeTOML_KeyOrder confirms tables keep the order their keys have
// in the document, in each form TOML can define them.
func TestDecodeTOML_KeyOrder(t *testing.T) {
	t.Run("top level and table", func(t *testing.T) {
		doc := "zebra = 1\nalpha = 2\n[tbl]\nz = 1\na = 2\n"
		assert.DeepEqual(t, tomlKeyOrder(t, doc), []string{"zebra", "alpha", "tbl"})
		assert.DeepEqual(t, tomlKeyOrder(t, doc, "tbl"), []string{"z", "a"})
	})

	t.Run("dotted keys", func(t *testing.T) {
		doc := "a.c = 1\na.b = 2\nz = 3\n"
		assert.DeepEqual(t, tomlKeyOrder(t, doc), []string{"a", "z"})
		assert.DeepEqual(t, tomlKeyOrder(t, doc, "a"), []string{"c", "b"})
	})

	t.Run("table defined in two places", func(t *testing.T) {
		doc := "[x.y]\nq = 1\n[x]\np = 2\n"
		assert.DeepEqual(t, tomlKeyOrder(t, doc, "x"), []string{"y", "p"})
	})

	t.Run("array of tables", func(t *testing.T) {
		doc := "[[arr]]\nn = 1\nm = 1\n[arr.sub]\nz = 1\na = 1\n[[arr]]\nb = 1\na = 1\n"
		assert.DeepEqual(t, tomlKeyOrder(t, doc, "arr", 0), []string{"n", "m", "sub"})
		assert.DeepEqual(t, tomlKeyOrder(t, doc, "arr", 0, "sub"), []string{"z", "a"})
		assert.DeepEqual(t, tomlKeyOrder(t, doc, "arr", 1), []string{"b", "a"})
	})

	t.Run("inline table", func(t *testing.T) {
		doc := "t = { k2 = 1, k1 = 2 }\n"
		assert.DeepEqual(t, tomlKeyOrder(t, doc, "t"), []string{"k2", "k1"})
	})
}

// TestDecodeTOML_InlineTablesInArray confirms a table inside an inline
// array has no recorded order, whatever the array's length, so it falls
// back to alphabetical order instead of getting a wrong or inconsistent
// one.
func TestDecodeTOML_InlineTablesInArray(t *testing.T) {
	for _, doc := range []string{
		"a = [{ z = 1, y = 2 }]\n",
		"a = [{ z = 1, y = 2 }, { x = 1, w = 2 }]\n",
	} {
		v, order, err := decodeTOML([]byte(doc))
		assert.NilError(t, err)

		for _, item := range v.(map[string]any)["a"].([]any) {
			_, ok := order.order(item.(map[string]any))
			assert.Assert(t, !ok, doc)
		}
	}
}
