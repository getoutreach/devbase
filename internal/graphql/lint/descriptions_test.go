// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Tests for the require-description and description-style
// Tier 3 rules.

// Many cases below are ported from @graphql-eslint/eslint-plugin's own
// test suite (packages/plugin/src/rules/require-description/index.test.ts
// and packages/plugin/src/rules/description-style/index.test.ts,
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

// enableRuleWithOptions returns a Lint with rule set to
// SeverityError and opts as its Rule.Options, the minimum
// scripts/devbase.yaml entry needed to turn on a Tier 3 rule that
// takes options.
func enableRuleWithOptions(rule string, opts map[string]any) *config.Lint {
	return &config.Lint{Rules: map[string]config.Rule{rule: {Severity: config.SeverityError, Options: opts}}}
}

// --- require-description: valid cases ported from
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

// --- require-description: invalid cases ported from
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

// --- require-description: disabled by default ---

func TestRequireDescriptionDisabledByDefault(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "schema.graphql", `type Query { a: String } type User { id: ID }`)

	violations, err := Files([]string{path}, nil)
	assert.NilError(t, err)
	assert.Equal(t, len(violations), 0)
}

// --- require-description: Go-specific edge cases ---

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

// --- description-style: cases ported from
// @graphql-eslint/eslint-plugin's description-style/index.test.ts ---

// descriptionStyleBlockSDL is @graphql-eslint's BLOCK_SDL fixture: 3
// block-string enum value descriptions.
const descriptionStyleBlockSDL = `
	type Query { a: String }
	enum EnumUserLanguagesSkill {
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
	}
`

// descriptionStyleInlineSDL is @graphql-eslint's INLINE_SDL fixture: an
// inline type description plus 2 inline field descriptions.
const descriptionStyleInlineSDL = `
	" Test "
	type CreateOneUserPayload {
		"Created document ID"
		recordId: ID

		"Created document"
		record: String
	}
`

func TestDescriptionStyleCases(t *testing.T) {
	cases := []struct {
		name            string
		sdl             string
		opts            map[string]any
		wantViolations  int
		wantMsgContains string
	}{
		{"block SDL, default block style, passes", descriptionStyleBlockSDL, nil, 0, ""},
		{"inline SDL, inline style, passes", descriptionStyleInlineSDL, map[string]any{"style": "inline"}, 0, ""},
		{
			"block SDL, inline style, reports every description", descriptionStyleBlockSDL,
			map[string]any{"style": "inline"},
			3, "Unexpected block description",
		},
		{
			"inline SDL, default block style, reports every description", descriptionStyleInlineSDL,
			nil, 3, "Unexpected inline description",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			path := writeFile(t, dir, "schema.graphql", c.sdl)

			violations, err := Files([]string{path}, enableRuleWithOptions(config.RuleDescriptionStyle, c.opts))
			assert.NilError(t, err)
			assert.Equal(t, len(violations), c.wantViolations)
			for _, v := range violations {
				assert.Equal(t, v.Rule, config.RuleDescriptionStyle)
				assert.ErrorContains(t, v.err, c.wantMsgContains)
			}
		})
	}
}

func TestDescriptionStyleDisabledByDefault(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "schema.graphql", descriptionStyleInlineSDL)

	violations, err := Files([]string{path}, nil)
	assert.NilError(t, err)
	assert.Equal(t, len(violations), 0)
}

// --- description-style: Go-specific edge cases ---

// TestDescriptionStyleIgnoresDefaultValueStrings confirms a field's
// string default value is never mistaken for a description, even
// though it is itself a String token sitting right next to a
// description-bearing field -- descriptionTokens must exclude it via
// the "=" it follows, not merely by position.
func TestDescriptionStyleIgnoresDefaultValueStrings(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "schema.graphql", `
		type Query { a: String }
		input Foo {
			bar: String = "default"
			"""
			baz description
			"""
			baz: String
		}
	`)

	violations, err := Files([]string{path}, enableRuleWithOptions(config.RuleDescriptionStyle, map[string]any{"style": "inline"}))
	assert.NilError(t, err)
	assert.Equal(t, len(violations), 1)
	assert.ErrorContains(t, violations[0].err, `Unexpected block description for input value "baz" in input "Foo"`)
}

// TestDescriptionStyleIgnoresDirectiveUsageArgumentStrings confirms a
// directive usage's own string argument (as opposed to a directive
// DEFINITION's argument) is never mistaken for a description, using
// the exact "@key(fields: \"...\")" shape a federated schema's
// `extend schema @link(...)` and `@key(...)` usages both produce.
func TestDescriptionStyleIgnoresDirectiveUsageArgumentStrings(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "schema.graphql", `
		extend schema @link(url: "https://specs.apollo.dev/federation/v2.3", import: ["@key"])

		type Widget @key(fields: "id") {
			"""
			Widget ID
			"""
			id: ID!
		}
	`)

	cfg := enableRuleWithOptions(config.RuleDescriptionStyle, nil)
	cfg.Federation = "v2.3"
	violations, err := Files([]string{path}, cfg)
	assert.NilError(t, err)
	assert.Equal(t, len(violations), 0)
}

// TestDescriptionStyleIgnoresNestedDefaultValueObjectAndList confirms
// string literals nested inside a list or input object default value
// are excluded too, not just a bare scalar default.
func TestDescriptionStyleIgnoresNestedDefaultValueObjectAndList(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "schema.graphql", `
		input Filter { x: String, y: [String] }
		type Foo {
			"""
			bar description
			"""
			bar(filter: Filter = { x: "a", y: ["b", "c"] }): String
		}
	`)

	violations, err := Files([]string{path}, enableRuleWithOptions(config.RuleDescriptionStyle, nil))
	assert.NilError(t, err)
	assert.Equal(t, len(violations), 0)
}

// TestDescriptionStyleMultiLineBlockDescriptionReportsCorrectPosition
// confirms the reported line is where the description starts, not
// where it ends, and the column is never negative -- see
// descriptionTokens' comment on gqlparser/v2's BlockString position
// bug.
func TestDescriptionStyleMultiLineBlockDescriptionReportsCorrectPosition(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "schema.graphql", "type Query { a: String }\n"+
		"type Foo {\n"+
		"  \"\"\"\n"+
		"  multi\n"+
		"  line\n"+
		"  \"\"\"\n"+
		"  bar: String\n"+
		"}\n")

	cfg := enableRuleWithOptions(config.RuleDescriptionStyle, map[string]any{"style": "inline"})
	violations, err := Files([]string{path}, cfg)
	assert.NilError(t, err)
	assert.Equal(t, len(violations), 1)
	// Line 3 is where the opening """ starts; column 3 is where it
	// starts on that line (2 leading spaces of indentation).
	assert.ErrorContains(t, violations[0].err, ":3:3: Unexpected block description")
}

// TestDescriptionStyleFieldArgumentDescriptionChecked confirms a field
// argument's own description -- distinct from any default value on
// that same argument -- is still matched to the right token.
func TestDescriptionStyleFieldArgumentDescriptionChecked(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "schema.graphql", `
		type Query {
			widgets(
				"""
				Maximum number of widgets to return
				"""
				limit: Int = 10
			): String
		}
	`)

	violations, err := Files([]string{path}, enableRuleWithOptions(config.RuleDescriptionStyle, map[string]any{"style": "inline"}))
	assert.NilError(t, err)
	assert.Equal(t, len(violations), 1)
	assert.ErrorContains(t, violations[0].err, `Unexpected block description for input value "limit" in field "widgets"`)
}

// TestDescriptionStyleExtensionFieldInDifferentFileFromBaseType confirms
// a description on a field (and that field's own argument) added via
// "extend type" in one file is matched to the right token even when the
// base type is defined in a different file -- see groupDescriptionSites'
// doc.Extensions loop for why a merged field keeps its own Position.Src.
// This is the shape of giraffe's real schema, e.g. "extend type Query {
// accounts(...) }" in one module file, with "type Query" itself defined
// elsewhere.
func TestDescriptionStyleExtensionFieldInDifferentFileFromBaseType(t *testing.T) {
	dir := t.TempDir()
	basePath := writeFile(t, dir, "base.graphql", `type Query { a: String }`)
	extPath := writeFile(t, dir, "ext.graphql", `
		extend type Query {
			"""
			Widgets query
			"""
			widgets(
				"""
				Maximum number of widgets to return
				"""
				limit: Int
			): String
		}
	`)

	cfg := enableRuleWithOptions(config.RuleDescriptionStyle, map[string]any{"style": "inline"})
	violations, err := Files([]string{basePath, extPath}, cfg)
	assert.NilError(t, err)
	assert.Equal(t, len(violations), 2)
	for _, v := range violations {
		assert.Equal(t, v.Rule, config.RuleDescriptionStyle)
		assert.ErrorContains(t, v.err, "Unexpected block description")
	}
}

// TestDescriptionStyleAndRequireDescriptionBothRun confirms the two
// rules operate independently in the same Files call: a missing
// description trips require-description, and a present description in
// the wrong style trips description-style, in the same run.
func TestDescriptionStyleAndRequireDescriptionBothRun(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "schema.graphql", `
		"""
		Foo description
		"""
		type Foo {
			a: String
		}
	`)

	cfg := &config.Lint{Rules: map[string]config.Rule{
		config.RuleRequireDescription: {Severity: config.SeverityError, Options: map[string]any{"types": true, "FieldDefinition": true}},
		config.RuleDescriptionStyle:   {Severity: config.SeverityError, Options: map[string]any{"style": "inline"}},
	}}
	violations, err := Files([]string{path}, cfg)
	assert.NilError(t, err)
	assert.Equal(t, len(violations), 2)

	rules := make([]string, 0, len(violations))
	for _, v := range violations {
		rules = append(rules, v.Rule)
	}
	assert.Equal(t, len(rules), 2)
	assert.Equal(t, rules[0] != rules[1], true)
}
