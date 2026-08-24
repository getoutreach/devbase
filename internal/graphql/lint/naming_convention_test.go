// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Tests for the naming-convention Tier 3 rule.

// Cases below are ported from @graphql-eslint/eslint-plugin's own test
// suite (packages/plugin/tests/naming-convention.spec.ts,
// @graphql-eslint/eslint-plugin@3.13.1, the version giraffe's
// .eslintrc.js pins today), adapted to devbase's SDL-only scope: every
// case exercising an OperationDefinition, FragmentDefinition,
// VariableDefinition, or a field selection (all operation-only AST
// kinds that never appear in a standalone schema file) is dropped, and
// the "large graphql file" case is dropped since it depends on a large
// upstream mock fixture this package does not vendor -- the smaller
// cases already exercise the same option shapes. See this file's own
// package doc comment (naming_convention.go) for the selector keys
// this port recognizes.

package lint

import (
	"testing"

	"github.com/getoutreach/devbase/v2/internal/graphql/config"
	"gotest.tools/v3/assert"
)

func TestNamingConventionValidCases(t *testing.T) {
	cases := []struct {
		name string
		sdl  string
		opts map[string]any
	}{
		{"PascalCase type", `type B { test: String }`, map[string]any{"types": "PascalCase"}},
		{"snake_case type", `type my_test_6_t { test: String }`, map[string]any{"types": "snake_case"}},
		{"UPPER_CASE type", `type MY_TEST_6_T { test: String }`, map[string]any{"types": "UPPER_CASE"}},
		{
			// A single leading/trailing underscore, not a double one:
			// GraphQL reserves any "__"-prefixed name for introspection,
			// so gqlparser's Tier 1 validation would reject "__B" outright
			// -- before naming-convention (Tier 3) ever runs -- unlike
			// @graphql-eslint's own rule-tester, which parses this fixture
			// without validating it as a full schema.
			"leading/trailing underscores allowed",
			`type _B { _test_: String }`,
			map[string]any{
				"allowLeadingUnderscore": true, "allowTrailingUnderscore": true,
				"types": "PascalCase", "FieldDefinition": "camelCase",
			},
		},
		{"scalar PascalCase", `scalar BSONDecimal`, map[string]any{"types": "PascalCase"}},
		{"interface PascalCase", `interface B { test: String }`, map[string]any{"types": "PascalCase"}},
		{"enum type and value", `enum B { TEST }`, map[string]any{"types": "PascalCase", "EnumValueDefinition": "UPPER_CASE"}},
		{"input type and value", `input Test { item: String }`, map[string]any{"types": "PascalCase", "InputValueDefinition": "camelCase"}},
		{
			"style option object with suffix",
			`type TypeOne { aField: String } enum Z { VALUE_ONE VALUE_TWO }`,
			map[string]any{
				"types":               map[string]any{"style": "PascalCase"},
				"FieldDefinition":     map[string]any{"style": "camelCase", "suffix": "Field"},
				"EnumValueDefinition": map[string]any{"style": "UPPER_CASE", "suffix": ""},
			},
		},
		{
			"style option object with prefix",
			`type One { fieldA: String } enum Z { ENUM_VALUE_ONE ENUM_VALUE_TWO }`,
			map[string]any{
				"types":               map[string]any{"style": "PascalCase"},
				"FieldDefinition":     map[string]any{"style": "camelCase", "prefix": "field"},
				"EnumValueDefinition": map[string]any{"style": "UPPER_CASE", "prefix": "ENUM_VALUE_"},
			},
		},
		{
			"Query field selector overrides the general FieldDefinition rule",
			`type One { fieldA: String } type Query { QUERY_A(id: ID!): String }`,
			map[string]any{
				"FieldDefinition[parent.name.value=Query]": map[string]any{"style": "UPPER_CASE", "prefix": "QUERY"},
				"FieldDefinition":                          map[string]any{"style": "camelCase", "prefix": "field"},
			},
		},
		{
			"should ignore fields matching ignorePattern",
			`type Test { EU: ID EUIntlFlag: ID UPC: ID }`,
			map[string]any{
				"FieldDefinition": map[string]any{"style": "camelCase", "ignorePattern": "^(EU|UPC)"},
			},
		},
		// Each type below needs a field to be a valid schema definition
		// on its own (gqlparser's Tier 1 validation rejects a fieldless
		// object type) -- @graphql-eslint's own rule-tester has no such
		// requirement, so its fixture is just `type t`.
		{"should allow single letter for camelCase", `type t { f: String }`, map[string]any{"ObjectTypeDefinition": "camelCase"}},
		{"should allow single letter for PascalCase", `type T { f: String }`, map[string]any{"ObjectTypeDefinition": "PascalCase"}},
		{"should allow single letter for snake_case", `type t { f: String }`, map[string]any{"ObjectTypeDefinition": "snake_case"}},
		{"should allow single letter for UPPER_CASE", `type T { f: String }`, map[string]any{"ObjectTypeDefinition": "UPPER_CASE"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			path := writeFile(t, dir, "schema.graphql", c.sdl)

			violations, err := Files([]string{path}, enableRuleWithOptions(config.RuleNamingConvention, c.opts))
			assert.NilError(t, err)
			assert.Equal(t, len(violations), 0)
		})
	}
}

func TestNamingConventionInvalidCases(t *testing.T) {
	cases := []struct {
		name         string
		sdl          string
		opts         map[string]any
		wantMessages []string
	}{
		{
			"type and field must be PascalCase",
			`type b { test: String }`,
			map[string]any{"types": "PascalCase", "FieldDefinition": "PascalCase"},
			[]string{
				`Type "b" should be in PascalCase format`,
				`Field "test" should be in PascalCase format`,
			},
		},
		{
			// Single underscores, not double: see the "leading/trailing
			// underscores allowed" valid case above for why.
			"leading and trailing underscores disallowed by default",
			`type _b { test_: String }`,
			map[string]any{"allowLeadingUnderscore": false, "allowTrailingUnderscore": false},
			[]string{"Leading underscores are not allowed", "Trailing underscores are not allowed"},
		},
		{
			"scalar must be snake_case",
			`scalar BSONDecimal`,
			map[string]any{"ScalarTypeDefinition": "snake_case"},
			[]string{`Scalar "BSONDecimal" should be in snake_case format`},
		},
		{
			"enum type and value casing",
			`enum B { test }`,
			map[string]any{"EnumTypeDefinition": "camelCase", "EnumValueDefinition": "UPPER_CASE"},
			[]string{
				`Enumerator "B" should be in camelCase format`,
				`Enumeration value "test" should be in UPPER_CASE format`,
			},
		},
		{
			"input type casing plus leading underscore on its field",
			`input test { _Value: String }`,
			map[string]any{"types": "PascalCase", "InputValueDefinition": "snake_case"},
			[]string{
				`Input type "test" should be in PascalCase format`,
				`Input property "_Value" should be in snake_case format`,
				"Leading underscores are not allowed",
			},
		},
		{
			"required suffix",
			`type TypeOne { aField: String } enum Z { VALUE_ONE VALUE_TWO }`,
			map[string]any{
				"ObjectTypeDefinition": map[string]any{"style": "camelCase"},
				"FieldDefinition":      map[string]any{"style": "camelCase", "suffix": "AAA"},
				"EnumValueDefinition":  map[string]any{"style": "camelCase", "suffix": "ENUM"},
			},
			[]string{
				`Type "TypeOne" should be in camelCase format`,
				`Field "aField" should have "AAA" suffix`,
				`Enumeration value "VALUE_ONE" should have "ENUM" suffix`,
				`Enumeration value "VALUE_TWO" should have "ENUM" suffix`,
			},
		},
		{
			"required prefix",
			`type One { aField: String } enum Z { A_ENUM_VALUE_ONE VALUE_TWO }`,
			map[string]any{
				"ObjectTypeDefinition": map[string]any{"style": "PascalCase"},
				"FieldDefinition":      map[string]any{"style": "camelCase", "prefix": "Field"},
				"EnumValueDefinition":  map[string]any{"style": "UPPER_CASE", "prefix": "ENUM"},
			},
			[]string{
				`Field "aField" should have "Field" prefix`,
				`Enumeration value "A_ENUM_VALUE_ONE" should have "ENUM" prefix`,
				`Enumeration value "VALUE_TWO" should have "ENUM" prefix`,
			},
		},
		{
			"forbidden prefixes and suffixes, including the Query field selector",
			`type One { getFoo: String, queryBar: String } type Query { getA(id: ID!): String, queryB: String } extend type Query { getC: String }`,
			map[string]any{
				"ObjectTypeDefinition": map[string]any{"style": "PascalCase", "forbiddenPrefixes": []any{"On"}},
				"FieldDefinition": map[string]any{
					"style": "camelCase", "forbiddenPrefixes": []any{"foo", "bar"}, "forbiddenSuffixes": []any{"Foo"},
				},
				"FieldDefinition[parent.name.value=Query]": map[string]any{
					"style": "camelCase", "forbiddenPrefixes": []any{"get", "query"},
				},
			},
			[]string{
				`Type "One" should not have "On" prefix`,
				`Field "getFoo" should not have "Foo" suffix`,
				`Field "getA" should not have "get" prefix`,
				`Field "queryB" should not have "query" prefix`,
				`Field "getC" should not have "get" prefix`,
			},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			path := writeFile(t, dir, "schema.graphql", c.sdl)

			violations, err := Files([]string{path}, enableRuleWithOptions(config.RuleNamingConvention, c.opts))
			assert.NilError(t, err)
			assert.Equal(t, len(violations), len(c.wantMessages))
			for i, want := range c.wantMessages {
				assert.Equal(t, violations[i].Rule, config.RuleNamingConvention)
				assert.ErrorContains(t, violations[i].err, want)
			}
		})
	}
}

// TestNamingConventionMergesExtensionFieldsUnderBaseType is a
// Go-specific edge case: like no-typename-prefix
// (typename_prefix_test.go), an extension's fields are checked once,
// under its base type's own root-field selector, since
// validator.ValidateSchemaDocument has already merged them into the
// base Definition.Fields by the time Tier 3 runs (see descriptions.go).
func TestNamingConventionMergesExtensionFieldsUnderBaseType(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "schema.graphql", `
		type Query { a: String }
		extend type Query { getB: String }
	`)

	opts := map[string]any{
		"FieldDefinition[parent.name.value=Query]": map[string]any{"forbiddenPrefixes": []any{"get"}},
	}
	violations, err := Files([]string{path}, enableRuleWithOptions(config.RuleNamingConvention, opts))
	assert.NilError(t, err)
	assert.Equal(t, len(violations), 1)
	assert.ErrorContains(t, violations[0].err, `Field "getB" should not have "get" prefix`)
}

// TestNamingConventionIgnoresPreludeAndBuiltinNames is a Go-specific
// edge case: gqlparser injects its own built-in directive definitions
// (skip, include, deprecated) into every parsed schema. None of them
// were written in a repository file, so a PascalCase DirectiveDefinition
// rule -- which their lowercase names would otherwise fail -- must
// still report zero violations, confirming the prelude was excluded.
func TestNamingConventionIgnoresPreludeAndBuiltinNames(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "schema.graphql", `type Query { a: String }`)

	opts := map[string]any{"DirectiveDefinition": "PascalCase"}
	violations, err := Files([]string{path}, enableRuleWithOptions(config.RuleNamingConvention, opts))
	assert.NilError(t, err)
	assert.Equal(t, len(violations), 0)
}
