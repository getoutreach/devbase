// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Tests for the no-typename-prefix Tier 3 rule.

// Cases below are ported from @graphql-eslint/eslint-plugin's own test
// suite (packages/plugin/tests/no-typename-prefix.spec.ts,
// @graphql-eslint/eslint-plugin@3.13.1, a version still in real use
// downstream), dropping the "eslint-disable-next-line"
// valid case: devbase has no inline-suppression comment syntax
// anywhere in its lint package, and unlike no-hashtag-description's
// own such case (hashtag_test.go), that comment has nothing to do with
// no-typename-prefix's own logic, so there is nothing meaningful to
// invert it into.

package lint

import (
	"testing"

	"github.com/getoutreach/devbase/v2/internal/graphql/config"
	"gotest.tools/v3/assert"
)

func TestNoTypenamePrefixValidCases(t *testing.T) {
	cases := []struct {
		name string
		sdl  string
	}{
		{"object type field not prefixed", `type User { id: ID! }`},
		{"interface field not prefixed", `interface Node { id: ID! }`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			path := writeFile(t, dir, "schema.graphql", c.sdl)

			violations, err := Files([]string{path}, enableRule(config.RuleNoTypenamePrefix))
			assert.NilError(t, err)
			assert.Equal(t, len(violations), 0)
		})
	}
}

func TestNoTypenamePrefixInvalidCases(t *testing.T) {
	cases := []struct {
		name         string
		sdl          string
		wantMessages []string
	}{
		{
			"one field prefixed",
			`type User { userId: ID! }`,
			[]string{`Field "userId" starts with the name of the parent type "User"`},
		},
		{
			"two fields prefixed",
			`type User { userId: ID! userName: String! }`,
			[]string{
				`Field "userId" starts with the name of the parent type "User"`,
				`Field "userName" starts with the name of the parent type "User"`,
			},
		},
		{
			"interface field prefixed",
			`interface Node { nodeId: ID! }`,
			[]string{`Field "nodeId" starts with the name of the parent type "Node"`},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			path := writeFile(t, dir, "schema.graphql", c.sdl)

			violations, err := Files([]string{path}, enableRule(config.RuleNoTypenamePrefix))
			assert.NilError(t, err)
			assert.Equal(t, len(violations), len(c.wantMessages))
			for i, want := range c.wantMessages {
				assert.Equal(t, violations[i].Rule, config.RuleNoTypenamePrefix)
				assert.ErrorContains(t, violations[i].err, want)
			}
		})
	}
}

// TestNoTypenamePrefixCaseInsensitive ports no-typename-prefix's own
// case-insensitive comparison (`fieldName.toLowerCase().startsWith(
// lowerTypeName)`): a field's casing does not have to match its
// parent type's for the prefix to count.
func TestNoTypenamePrefixCaseInsensitive(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "schema.graphql", `type User { UserID: ID! }`)

	violations, err := Files([]string{path}, enableRule(config.RuleNoTypenamePrefix))
	assert.NilError(t, err)
	assert.Equal(t, len(violations), 1)
	assert.ErrorContains(t, violations[0].err, `Field "UserID" starts with the name of the parent type "User"`)
}

// TestNoTypenamePrefixChecksExtensionFieldsUnderBaseType is a
// Go-specific edge case: validator.ValidateSchemaDocument merges a type
// extension's fields into its base type's own Definition.Fields before
// Tier 3 ever runs (see descriptions.go), so an extension's field is
// checked against its base type's name, not walked (or missed)
// separately.
func TestNoTypenamePrefixChecksExtensionFieldsUnderBaseType(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "schema.graphql", `
		type User { id: ID! }
		extend type User { userName: String! }
	`)

	violations, err := Files([]string{path}, enableRule(config.RuleNoTypenamePrefix))
	assert.NilError(t, err)
	assert.Equal(t, len(violations), 1)
	assert.ErrorContains(t, violations[0].err, `Field "userName" starts with the name of the parent type "User"`)
}

// TestNoTypenamePrefixChecksDanglingExtensionFields is a Go-specific
// edge case: gqlparser never errors on an `extend type` whose base type
// is never defined anywhere (see directives.go's
// gapFillPossibleTypeExtension). It silently synthesizes an empty
// placeholder type instead, so Tier 3 still reaches the extension's
// own fields and must check them under the extension's own type name.
func TestNoTypenamePrefixChecksDanglingExtensionFields(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "schema.graphql", `extend type Ghost { ghostId: ID! }`)

	violations, err := Files([]string{path}, enableRule(config.RuleNoTypenamePrefix))
	assert.NilError(t, err)
	assert.Equal(t, len(violations), 1)
	assert.ErrorContains(t, violations[0].err, `Field "ghostId" starts with the name of the parent type "Ghost"`)
}
