// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Tests for the require-description Tier 3 rule.

// Many cases below are ported from @graphql-eslint/eslint-plugin's own
// test suite (packages/plugin/src/rules/require-description/index.test.ts,
// graphql-hive/graphql-eslint@master as of 2026-08-22), adapted to
// devbase's SDL-only scope: cases exercising OperationDefinition or
// ignoredSelectors are dropped, since devbase graphql lint has no
// notion of either -- it lints standalone *.graphql schema files, not
// operation documents, and require-description's Go port has no
// selector engine.

package lint

import (
	"testing"

	"github.com/getoutreach/devbase/v2/internal/graphql/config"
	"gotest.tools/v3/assert"
)

// --- valid cases ported from
// @graphql-eslint/eslint-plugin's require-description/index.test.ts ---

func TestRequireDescriptionValidCases(t *testing.T) {
	cases := []struct {
		name string
		sdl  string
		opts map[string]any
	}{
		{
			"all enum values described",
			`enum EnumUserLanguagesSkill {
				"""
				basic
				"""
				basic
				"""
				fluent
				"""
				fluent
				"""
				native
				"""
				native
			}`,
			map[string]any{"EnumValueDefinition": true},
		},
		{
			"all input values described",
			`input SalaryDecimalOperatorsFilterUpdateOneUserInput {
				"""
				gt
				"""
				gt: Float
				"""
				in
				"""
				in: [Float]
				" nin "
				nin: [Float]
			}`,
			map[string]any{"InputValueDefinition": true},
		},
		{
			"type and its fields described",
			`" Test "
			type CreateOneUserPayload {
				"Created document ID"
				recordId: ID

				"Created document"
				record: String
			}`,
			map[string]any{"types": true, "FieldDefinition": true},
		},
		{
			"root field described",
			`type Query {
				"Get user"
				user(id: ID!): String
			}`,
			map[string]any{"rootField": true},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			path := writeFile(t, dir, "schema.graphql", c.sdl)

			violations, err := Files([]string{path}, enableRuleWithOptions(config.RuleRequireDescription, c.opts))
			assert.NilError(t, err)
			assert.Equal(t, len(violations), 0)
		})
	}
}

// --- invalid cases ported from
// @graphql-eslint/eslint-plugin's require-description/index.test.ts ---

func TestRequireDescriptionInvalidCases(t *testing.T) {
	cases := []struct {
		name string
		sdl  string
		opts map[string]any
	}{
		{"ObjectTypeDefinition", `type User { id: ID }`, map[string]any{"ObjectTypeDefinition": true}},
		{"InterfaceTypeDefinition", `interface Node { id: ID! }`, map[string]any{"InterfaceTypeDefinition": true}},
		{"EnumTypeDefinition", `enum Role { ADMIN }`, map[string]any{"EnumTypeDefinition": true}},
		{"ScalarTypeDefinition", `scalar Email`, map[string]any{"ScalarTypeDefinition": true}},
		{"InputObjectTypeDefinition", `input CreateUserInput { email: String! }`, map[string]any{"InputObjectTypeDefinition": true}},
		{
			"UnionTypeDefinition",
			`type Book { id: ID! } type Movie { id: ID! } union Media = Book | Movie`,
			map[string]any{"UnionTypeDefinition": true},
		},
		{"DirectiveDefinition", `directive @auth(requires: String!) on FIELD_DEFINITION`, map[string]any{"DirectiveDefinition": true}},
		{"FieldDefinition", `type User { email: String! }`, map[string]any{"FieldDefinition": true}},
		{"InputValueDefinition", `input CreateUserInput { email: String! }`, map[string]any{"InputValueDefinition": true}},
		{"EnumValueDefinition", `enum Role { ADMIN }`, map[string]any{"EnumValueDefinition": true}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			path := writeFile(t, dir, "schema.graphql", c.sdl)

			violations, err := Files([]string{path}, enableRuleWithOptions(config.RuleRequireDescription, c.opts))
			assert.NilError(t, err)
			assert.Equal(t, len(violations), 1)
			assert.Equal(t, violations[0].Rule, config.RuleRequireDescription)
		})
	}
}

// TestRequireDescriptionPerKindOverrideDisablesObjectTypeDefinition
// ports "should disable description for ObjectTypeDefinition":
// types:true enables all 6 TYPES_KINDS, but an explicit
// ObjectTypeDefinition:false carves object types back out, while
// FieldDefinition:true still requires one on every field -- so only
// the 2 undescribed fields are flagged, not the undescribed type.
func TestRequireDescriptionPerKindOverrideDisablesObjectTypeDefinition(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "schema.graphql", `
		type CreateOneUserPayload {
			recordId: ID
			record: String
		}
	`)

	cfg := enableRuleWithOptions(config.RuleRequireDescription, map[string]any{
		"types": true, "ObjectTypeDefinition": false, "FieldDefinition": true,
	})
	violations, err := Files([]string{path}, cfg)
	assert.NilError(t, err)
	assert.Equal(t, len(violations), 2)
	for _, v := range violations {
		assert.Equal(t, v.Rule, config.RuleRequireDescription)
	}
}

func TestRequireDescriptionRootFieldMissingFails(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "schema.graphql", `type Query { user(id: String!): String! }`)

	violations, err := Files([]string{path}, enableRuleWithOptions(config.RuleRequireDescription, map[string]any{"rootField": true}))
	assert.NilError(t, err)
	assert.Equal(t, len(violations), 1)
	assert.ErrorContains(t, violations[0].err, `Description is required for field "user" in type "Query"`)
}

func TestRequireDescriptionRootFieldViaExplicitSchemaBlock(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "schema.graphql", `
		schema { query: Query, mutation: MyMutation }
		type Query {
			"A"
			a: String
		}
		type MyMutation {
			createUser(id: [ID!]!): String!
		}
	`)

	violations, err := Files([]string{path}, enableRuleWithOptions(config.RuleRequireDescription, map[string]any{"rootField": true}))
	assert.NilError(t, err)
	assert.Equal(t, len(violations), 1)
	assert.ErrorContains(t, violations[0].err, `Description is required for field "createUser" in type "MyMutation"`)
}

func TestRequireDescriptionDisabledByDefault(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "schema.graphql", `type Query { a: String } type User { id: ID }`)

	violations, err := Files([]string{path}, nil)
	assert.NilError(t, err)
	assert.Equal(t, len(violations), 0)
}

// --- Go-specific edge cases ---

// TestRequireDescriptionInputObjectFieldUsesInputValueDefinitionNotField
// confirms that, unlike graphql-js (which has a distinct
// InputValueDefinition AST node for an input object's own fields),
// gqlparser represents them with the same FieldDefinition struct it
// uses for an object type's fields -- so the Go port must still bucket
// an input object's field under the InputValueDefinition option, not
// FieldDefinition, to match @graphql-eslint's behavior.
func TestRequireDescriptionInputObjectFieldUsesInputValueDefinitionNotField(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "schema.graphql", `
		input CreateUserInput { email: String! }
	`)

	// FieldDefinition alone must not require a description on the input
	// object's field.
	violations, err := Files([]string{path}, enableRuleWithOptions(config.RuleRequireDescription, map[string]any{"FieldDefinition": true}))
	assert.NilError(t, err)
	assert.Equal(t, len(violations), 0)

	// InputValueDefinition alone must.
	violations, err = Files([]string{path}, enableRuleWithOptions(config.RuleRequireDescription, map[string]any{"InputValueDefinition": true}))
	assert.NilError(t, err)
	assert.Equal(t, len(violations), 1)
	assert.ErrorContains(t, violations[0].err, `Description is required for input value "email" in input "CreateUserInput"`)
}

// TestRequireDescriptionExtensionFieldsChecked confirms a field added
// via "extend type" is still checked, even though the extension itself
// can never carry a description (the grammar forbids one) and so is
// never itself flagged even with types:true.
func TestRequireDescriptionExtensionFieldsChecked(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "schema.graphql", `
		type Foo { a: String }
		extend type Foo {
			b: String
		}
	`)

	cfg := enableRuleWithOptions(config.RuleRequireDescription, map[string]any{"types": true, "FieldDefinition": true})
	violations, err := Files([]string{path}, cfg)
	assert.NilError(t, err)
	assert.Equal(t, len(violations), 3) // Foo's own missing description, plus fields "a" and "b".
}

// TestRequireDescriptionWhitespaceOnlyDescriptionFails confirms a
// description that is present but blank once trimmed still counts as
// missing, matching @graphql-eslint's own `.trim()` check.
func TestRequireDescriptionWhitespaceOnlyDescriptionFails(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "schema.graphql", `
		"   "
		type Foo { a: String }
	`)

	violations, err := Files([]string{path}, enableRuleWithOptions(config.RuleRequireDescription, map[string]any{"types": true}))
	assert.NilError(t, err)
	assert.Equal(t, len(violations), 1)
	assert.ErrorContains(t, violations[0].err, `Description is required for type "Foo"`)
}

// TestRequireDescriptionDirectiveArgumentChecked confirms a directive
// definition's own arguments are checked under InputValueDefinition,
// the same bucket a field argument uses.
func TestRequireDescriptionDirectiveArgumentChecked(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "schema.graphql", `
		directive @auth(requires: String!) on FIELD_DEFINITION
	`)

	cfg := enableRuleWithOptions(config.RuleRequireDescription, map[string]any{"InputValueDefinition": true})
	violations, err := Files([]string{path}, cfg)
	assert.NilError(t, err)
	assert.Equal(t, len(violations), 1)
	assert.ErrorContains(t, violations[0].err, `Description is required for input value "requires" in directive "auth"`)
}
