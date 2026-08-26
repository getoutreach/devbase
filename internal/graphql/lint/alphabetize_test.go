// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Tests for the alphabetize Tier 3 rule.

// Cases below are ported from @graphql-eslint/eslint-plugin's own test
// suite (packages/plugin/tests/alphabetize.spec.ts,
// @graphql-eslint/eslint-plugin@3.13.1, the version this port targets),
// dropping every case exercising "selections",
// "variables", or "definitions": those options only apply to operation
// documents or top-level document ordering, neither of which devbase
// graphql lint parses (see alphabetize.go's package doc comment). The
// "should move comment" case is also dropped: it exercises
// @graphql-eslint's --fix comment relocation, which has no Go
// counterpart and asserts the same violation messages as the plain
// ObjectTypeDefinition case already ported below. The two argument-order
// cases replace their original undefined argument types (Cc, Bb, Aa)
// with Int: @graphql-eslint's rule tester never checks that a type
// actually exists, but gqlparser does as part of devbase's Tier 1 pass,
// ahead of Tier 3 ever running.

package lint

import (
	"testing"

	"github.com/getoutreach/devbase/v2/internal/graphql/config"
	"gotest.tools/v3/assert"
)

func TestAlphabetizeValidCases(t *testing.T) {
	cases := []struct {
		name string
		sdl  string
		opts map[string]any
	}{
		{
			"sorted object fields",
			`type User { age: Int firstName: String! lastName: String! password: String }`,
			map[string]any{"fields": []any{"ObjectTypeDefinition"}},
		},
		{
			"sorted input fields",
			`input UserInput { age: Int firstName: String! lastName: String! password: String zip: String }`,
			map[string]any{"fields": []any{"InputObjectTypeDefinition"}},
		},
		{
			"sorted enum values",
			`enum Role { ADMIN GOD SUPER_ADMIN USER }`,
			map[string]any{"values": []any{"EnumTypeDefinition"}},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			path := writeFile(t, dir, "schema.graphql", c.sdl)

			violations, err := Files([]string{path}, enableRuleWithOptions(config.RuleAlphabetize, c.opts))
			assert.NilError(t, err)
			assert.Equal(t, len(violations), 0)
		})
	}
}

func TestAlphabetizeInvalidCases(t *testing.T) {
	withOpt := func(key, kind string) *config.Lint {
		return enableRuleWithOptions(config.RuleAlphabetize, map[string]any{key: []any{kind}})
	}
	values := enableRuleWithOptions(config.RuleAlphabetize, map[string]any{"values": []any{"EnumTypeDefinition"}})

	runOrderedViolationCases(t, config.RuleAlphabetize, []orderedViolationCase{
		{
			"unsorted object fields",
			`type User { password: String firstName: String! age: Int lastName: String! }`,
			withOpt("fields", "ObjectTypeDefinition"),
			[]string{"`firstName` should be before `password`.", "`age` should be before `firstName`."},
		},
		{
			"unsorted object extension fields",
			`extend type User { age: Int firstName: String! password: String lastName: String! }`,
			withOpt("fields", "ObjectTypeDefinition"),
			[]string{"`lastName` should be before `password`."},
		},
		{
			"unsorted interface fields",
			`interface Test { cc: Int bb: Int aa: Int }`,
			withOpt("fields", "InterfaceTypeDefinition"),
			[]string{"`bb` should be before `cc`.", "`aa` should be before `bb`."},
		},
		{
			"unsorted input fields",
			`input UserInput { password: String firstName: String! age: Int lastName: String! }`,
			withOpt("fields", "InputObjectTypeDefinition"),
			[]string{"`firstName` should be before `password`.", "`age` should be before `firstName`."},
		},
		{
			"unsorted input extension fields",
			`extend input UserInput { age: Int firstName: String! password: String lastName: String! }`,
			withOpt("fields", "InputObjectTypeDefinition"),
			[]string{"`lastName` should be before `password`."},
		},
		{
			"unsorted enum values",
			`enum Role { SUPER_ADMIN ADMIN USER GOD }`,
			values,
			[]string{"`ADMIN` should be before `SUPER_ADMIN`.", "`GOD` should be before `USER`."},
		},
		{
			"unsorted enum extension values",
			`extend enum Role { ADMIN SUPER_ADMIN GOD USER }`,
			values,
			[]string{"`GOD` should be before `SUPER_ADMIN`."},
		},
		{
			"should compare with lexicographic order",
			`enum Test { qux foo Bar bar }`,
			values,
			[]string{"`foo` should be before `qux`.", "`Bar` should be before `foo`.", "`bar` should be before `Bar`."},
		},
		{
			"unsorted directive definition arguments",
			`directive @test(cc: Int, bb: Int, aa: Int) on FIELD_DEFINITION`,
			withOpt("arguments", "DirectiveDefinition"),
			[]string{"`bb` should be before `cc`.", "`aa` should be before `bb`."},
		},
		{
			"unsorted field arguments",
			`type Query { test(cc: Int, bb: Int, aa: Int): Int }`,
			withOpt("arguments", "FieldDefinition"),
			[]string{"`bb` should be before `cc`.", "`aa` should be before `bb`."},
		},
	})
}

// TestAlphabetizeOrdersExtensionFieldsWithBaseType is a Go-specific edge
// case: validator.ValidateSchemaDocument merges a type extension's
// fields into its base type's own Definition.Fields before Tier 3 ever
// runs (see alphabetize.go), so a base type and its extension are
// checked as one alphabetical run -- even when each is internally
// sorted on its own, an extension whose fields sort before the base
// type's own is a violation. @graphql-eslint itself cannot see this
// case: it lints one file's own AST at a time, so it would report
// neither half as a violation.
func TestAlphabetizeOrdersExtensionFieldsWithBaseType(t *testing.T) {
	dir := t.TempDir()
	base := writeFile(t, dir, "base.graphql", `type User { b: String d: String }`)
	ext := writeFile(t, dir, "ext.graphql", `extend type User { a: String c: String }`)

	opts := map[string]any{"fields": []any{"ObjectTypeDefinition"}}
	violations, err := Files([]string{base, ext}, enableRuleWithOptions(config.RuleAlphabetize, opts))
	assert.NilError(t, err)
	assert.Equal(t, len(violations), 1)
	assert.Equal(t, violations[0].Rule, config.RuleAlphabetize)
	assert.ErrorContains(t, violations[0].err, "`a` should be before `d`.")
}
