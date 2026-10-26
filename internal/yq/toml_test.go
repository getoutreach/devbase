// Copyright 2026 Outreach Corporation. Licensed under the Apache License 2.0.

// Description: Tests for TOML decoding and tomlq flag validation.

package yq

import (
	"encoding/json"
	"testing"

	"gotest.tools/v3/assert"
)

// TestDecodeTOML confirms decodeTOML produces the plain types gojq
// operates on: json.Number for numbers (never float64 or int64), and
// strings for every date and time form.
func TestDecodeTOML(t *testing.T) {
	v, err := decodeTOML([]byte(`
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

// TestDecodeTOML_Invalid confirms a syntax error is reported rather
// than yielding a partial document.
func TestDecodeTOML_Invalid(t *testing.T) {
	_, err := decodeTOML([]byte("a = \n"))
	assert.ErrorContains(t, err, "decode toml")
}

// TestDecodeTOML_NonFiniteNumber confirms a TOML inf is not wrapped as
// a json.Number holding invalid JSON text, so Engine.Run rejects it.
func TestDecodeTOML_NonFiniteNumber(t *testing.T) {
	v, err := decodeTOML([]byte("x = inf\n"))
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
