// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Tests for the description-style Tier 3 rule.

// Cases ported from @graphql-eslint/eslint-plugin's own test suite
// (packages/plugin/src/rules/description-style/index.test.ts,
// graphql-hive/graphql-eslint@master as of 2026-08-22) are marked as
// such; the rest are Go-specific edge cases.

package lint

import (
	"testing"

	"github.com/getoutreach/devbase/v2/internal/graphql/config"
	"gotest.tools/v3/assert"
)

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

// --- cases ported from
// @graphql-eslint/eslint-plugin's description-style/index.test.ts ---

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

// TestDescriptionStyleSkipsFilesWithNoDescriptions confirms that a
// file contributing zero non-empty description sites is skipped
// without error or violations, and does not affect the violations a
// sibling file's real descriptions still produce.
func TestDescriptionStyleSkipsFilesWithNoDescriptions(t *testing.T) {
	dir := t.TempDir()
	emptyPath := writeFile(t, dir, "empty.graphql", `type Query { a: String }`)
	blockPath := writeFile(t, dir, "block.graphql", descriptionStyleBlockSDL)

	cfg := enableRuleWithOptions(config.RuleDescriptionStyle, map[string]any{"style": "inline"})
	violations, err := Files([]string{emptyPath, blockPath}, cfg)
	assert.NilError(t, err)
	for _, v := range violations {
		assert.Equal(t, v.File(), blockPath)
	}
}

// --- Go-specific edge cases ---

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
// This is a common real-world shape: a module extends the root Query
// type with its own fields in a separate file from where Query itself
// is defined.
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
