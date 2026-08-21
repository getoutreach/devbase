// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Tests for Tier 1 rule classification and *.graphql file
// discovery.

package lint

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/getoutreach/devbase/v2/internal/graphql/config"
	"gotest.tools/v3/assert"
)

// writeFile writes contents to path relative to dir, creating parent
// directories as needed.
func writeFile(t *testing.T, dir, path, contents string) string {
	t.Helper()
	full := filepath.Join(dir, path)
	assert.NilError(t, os.MkdirAll(filepath.Dir(full), 0o755))
	assert.NilError(t, os.WriteFile(full, []byte(contents), 0o600))
	return full
}

// tier1Case is a single Tier 1 rule's minimal SDL fixture: sdl violates
// exactly the rule under test, and wantErrSubstring is a substring of
// the resulting Violation's error confirming it failed for that rule,
// not some other validation.
type tier1Case struct {
	rule             string
	sdl              string
	wantErrSubstring string
}

// TestLintFilesClassifiesTier1Rules empirically confirms, for each of
// the 10 Tier 1 rules, that Files both surfaces a gqlparser parse error
// for a minimal SDL fixture violating it and tags the resulting
// Violation with that rule's name, using the gqlparser/v2 version
// pinned in go.mod.
func TestLintFilesClassifiesTier1Rules(t *testing.T) {
	cases := []tier1Case{
		{
			rule: config.RuleUniqueDirectiveNames,
			sdl: `
				directive @foo on FIELD_DEFINITION
				directive @foo on FIELD_DEFINITION
			`,
			wantErrSubstring: "Cannot redeclare directive foo.",
		},
		{
			rule: config.RuleUniqueFieldDefinitionNames,
			sdl: `
				type Foo {
					bar: String
					bar: Int
				}
			`,
			wantErrSubstring: "Field Foo.bar can only be defined once.",
		},
		{
			rule: config.RuleUniqueTypeNames,
			sdl: `
				type Foo { a: String }
				type Foo { b: String }
			`,
			wantErrSubstring: "Cannot redeclare type Foo.",
		},
		{
			rule: config.RuleUniqueEnumValueNames,
			sdl: `
				enum Foo {
					BAR
					BAR
				}
			`,
			wantErrSubstring: "Enum value Foo.BAR can only be defined once.",
		},
		{
			rule: config.RuleKnownArgumentNames,
			sdl: `
				directive @foo(arg: String) on FIELD_DEFINITION
				type Foo {
					bar: String @foo(argX: "test")
				}
			`,
			wantErrSubstring: "Undefined argument argX for directive foo.",
		},
		{
			rule: config.RuleKnownDirectives,
			sdl: `
				type Foo {
					bar: String @undefinedDirective
				}
			`,
			wantErrSubstring: "Undefined directive undefinedDirective.",
		},
		{
			rule: config.RuleKnownTypeNames,
			sdl: `
				type Foo {
					bar: UndefinedType
				}
			`,
			wantErrSubstring: "Undefined type UndefinedType.",
		},
		{
			rule: config.RuleProvidedRequiredArguments,
			sdl: `
				directive @foo(arg: String!) on FIELD_DEFINITION
				type Foo {
					bar: String @foo
				}
			`,
			wantErrSubstring: "Argument arg for directive foo cannot be null.",
		},
		{
			// gqlparser rejects any second schema { } block outright,
			// regardless of whether its root operations overlap with the
			// first block's. ruleForMessage tags this as
			// lone-schema-definition rather than unique-operation-types --
			// gqlparser raises the identical message for a genuine
			// cross-block operation-type duplicate, and the two are not
			// distinguishable from the message alone.
			rule: config.RuleLoneSchemaDefinition,
			sdl: `
				schema { query: Query }
				schema { mutation: Mutation }
				type Query { a: String }
				type Mutation { a: String }
			`,
			wantErrSubstring: "Cannot have multiple schema entry points",
		},
	}

	for _, c := range cases {
		t.Run(c.rule, func(t *testing.T) {
			dir := t.TempDir()
			path := writeFile(t, dir, "schema.graphql", c.sdl)

			violations, err := Files([]string{path}, nil)
			assert.NilError(t, err)
			assert.Equal(t, len(violations), 1)
			assert.Equal(t, violations[0].Rule, c.rule)
			assert.ErrorContains(t, violations[0].err, c.wantErrSubstring)
		})
	}
}

// TestLintFilesUniqueOperationTypesWithinSingleSchemaBlockGap documents
// a gap in the unique-operation-types rule: gqlparser only rejects
// multiple schema { } blocks. Two "query:" entries inside a single block
// parse without error, and gqlparser silently keeps the last one, so
// Files reports no violation here. A Tier 2 gap-fill pass is
// required to flag this case (matching @graphql-eslint's behavior).
func TestLintFilesUniqueOperationTypesWithinSingleSchemaBlockGap(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "schema.graphql", `
		schema {
			query: Query
			query: OtherQuery
		}
		type Query { a: String }
		type OtherQuery { a: String }
	`)

	violations, err := Files([]string{path}, nil)
	assert.NilError(t, err)
	assert.Equal(t, len(violations), 0)
}

// TestLintFilesCombinesMultipleFilesIntoOneSchema confirms that
// Files parses its inputs as a single schema, so a type defined in
// one file is visible when validating a reference to it in another.
func TestLintFilesCombinesMultipleFilesIntoOneSchema(t *testing.T) {
	dir := t.TempDir()
	typesPath := writeFile(t, dir, "types.graphql", `type Foo { a: String }`)
	queryPath := writeFile(t, dir, "query.graphql", `type Query { foo: Foo }`)

	violations, err := Files([]string{typesPath, queryPath}, nil)
	assert.NilError(t, err)
	assert.Equal(t, len(violations), 0)
}

// TestLintFilesUnclassifiedRule confirms that a gqlparser schema error
// outside the 9 named Tier 1 rules -- here, the reserved "__" name
// prefix gqlparser also rejects -- is still reported, tagged with
// UnclassifiedRule rather than dropped.
func TestLintFilesUnclassifiedRule(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "schema.graphql", `type Foo { __bar: String }`)

	violations, err := Files([]string{path}, nil)
	assert.NilError(t, err)
	assert.Equal(t, len(violations), 1)
	assert.Equal(t, violations[0].Rule, UnclassifiedRule)
}

func TestFindGraphQLFilesSkipsExcludedAndNonGraphQLFiles(t *testing.T) {
	dir := t.TempDir()
	included := writeFile(t, dir, "schema/types.graphql", "type Foo { a: String }")
	writeFile(t, dir, "schema/shared.graphql", "type Bar { a: String }")
	writeFile(t, dir, "schema/nested/shared.graphql", "type Baz { a: String }")
	writeFile(t, dir, "schema/README.md", "not graphql")

	got, err := FindGraphQLFiles([]string{dir}, []string{"**/shared.graphql"})
	assert.NilError(t, err)
	assert.DeepEqual(t, got, []string{included})
}

func TestFindGraphQLFilesAcceptsADirectFilePath(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "schema.graphql", "type Foo { a: String }")

	got, err := FindGraphQLFiles([]string{path}, nil)
	assert.NilError(t, err)
	assert.DeepEqual(t, got, []string{path})
}

func TestMatchGlobDoublestarMatchesAnyDepth(t *testing.T) {
	cases := []struct {
		pattern string
		path    string
		want    bool
	}{
		{"**/shared.graphql", "shared.graphql", true},
		{"**/shared.graphql", "a/b/shared.graphql", true},
		{"**/shared.graphql", "shared.graphql.bak", false},
		{"**/shared.graphql", "sharedx.graphql", false},
		{"internal/*.graphql", "internal/foo.graphql", true},
		{"internal/*.graphql", "internal/nested/foo.graphql", false},
	}

	for _, c := range cases {
		got := matchGlob(c.pattern, c.path)
		assert.Equal(t, got, c.want, "matchGlob(%q, %q)", c.pattern, c.path)
	}
}
