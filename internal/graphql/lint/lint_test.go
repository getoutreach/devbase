// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Tests for Tier 1 rule classification and *.graphql file
// discovery.

package lint

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/getoutreach/devbase/v2/internal/graphql/config"
	"github.com/vektah/gqlparser/v2/ast"
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

// singleViolationCase is a Tier 3 rule's minimal, independently-valid SDL
// fixture: sdl violates exactly the rule under test, in isolation, and is
// expected to produce exactly one violation of it.
type singleViolationCase struct {
	name string
	sdl  string
}

// runSingleViolationCases runs cases against Files with rule enabled
// (enableRule), asserting each produces exactly one violation tagged rule
// whose error contains wantErrSubstring. It is shared by any Tier 3 rule
// whose invalid cases all report the same message shape.
func runSingleViolationCases(t *testing.T, rule, wantErrSubstring string, cases []singleViolationCase) {
	t.Helper()
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			path := writeFile(t, dir, "schema.graphql", c.sdl)

			violations, err := Files([]string{path}, enableRule(rule))
			assert.NilError(t, err)
			assert.Equal(t, len(violations), 1)
			assert.Equal(t, violations[0].Rule, rule)
			assert.ErrorContains(t, violations[0].err, wantErrSubstring)
		})
	}
}

// orderedViolationCase is a Tier 3 rule's SDL fixture, the Lint config
// to run it under, and the sequence of violation messages Files is
// expected to report, in order.
type orderedViolationCase struct {
	name         string
	sdl          string
	cfg          *config.Lint
	wantMessages []string
}

// runOrderedViolationCases runs cases against Files, asserting the
// resulting violations match wantMessages in order and are all tagged
// rule. Unlike runSingleViolationCases, it is shared by any Tier 3 rule
// whose invalid cases report more than one violation in a fixed order.
func runOrderedViolationCases(t *testing.T, rule string, cases []orderedViolationCase) {
	t.Helper()
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			path := writeFile(t, dir, "schema.graphql", c.sdl)

			violations, err := Files([]string{path}, c.cfg)
			assert.NilError(t, err)
			assert.Equal(t, len(violations), len(c.wantMessages))
			for i, want := range c.wantMessages {
				assert.Equal(t, violations[i].Rule, rule)
				assert.ErrorContains(t, violations[i].err, want)
			}
		})
	}
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
			// first block's.
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

// TestLintFilesUniqueOperationTypesWithinSingleSchemaBlock confirms that
// two "query:" entries inside a single schema { } block are reported as
// a unique-operation-types violation (gqlparser v2.5.63 and later).
func TestLintFilesUniqueOperationTypesWithinSingleSchemaBlock(t *testing.T) {
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
	assert.Equal(t, len(violations), 1)
	assert.Equal(t, violations[0].Rule, config.RuleUniqueOperationTypes)
	assert.ErrorContains(t, violations[0].err, "There can be only one query type in schema.")
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

// TestLintFilesSkipsOperationDocuments confirms that a file holding only
// executable definitions (operations and fragments, such as a genqlient
// input) is not linted as schema SDL.
func TestLintFilesSkipsOperationDocuments(t *testing.T) {
	operations := map[string]string{
		"query":        `query Foo($id: ID!) { foo(id: $id) { a } }`,
		"mutation":     `mutation Foo { setFoo { a } }`,
		"subscription": `subscription Foo { foo { a } }`,
		"fragment":     `fragment FooParts on Foo { a }`,
		"shorthand":    `{ foo { a } }`,
	}
	for name, doc := range operations {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			schemaPath := writeFile(t, dir, "schema.graphql", `type Foo { a: String }`)
			opPath := writeFile(t, dir, "op.graphql", doc)

			violations, err := Files([]string{schemaPath, opPath}, nil)
			assert.NilError(t, err)
			assert.Equal(t, len(violations), 0)
		})
	}
}

// TestLintFilesReportsMixedOperationAndSchemaDocument confirms that a
// file mixing schema and executable definitions is not skipped: it is
// valid as neither a schema nor an operation document.
func TestLintFilesReportsMixedOperationAndSchemaDocument(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "mixed.graphql", "type Foo { a: String }\nquery Q { a }")

	violations, err := Files([]string{path}, nil)
	assert.NilError(t, err)
	assert.Equal(t, len(violations), 1)
	assert.Equal(t, violations[0].Rule, UnclassifiedRule)
}

// TestWithoutOperationDocumentsKeepsSchemaAndEmptySources confirms that
// only sources with at least one operation or fragment are dropped:
// schema, empty, and comment-only sources are kept, in order.
func TestWithoutOperationDocumentsKeepsSchemaAndEmptySources(t *testing.T) {
	schema := &ast.Source{Name: "schema.graphql", Input: `type Foo { a: String }`}
	empty := &ast.Source{Name: "empty.graphql", Input: ""}
	comment := &ast.Source{Name: "comment.graphql", Input: "# nothing here"}
	operation := &ast.Source{Name: "op.graphql", Input: `query Q { a }`}

	got := withoutOperationDocuments([]*ast.Source{schema, operation, empty, comment})
	assert.DeepEqual(t, got, []*ast.Source{schema, empty, comment})
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
