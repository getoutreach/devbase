// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Tests for the unique-directive-names-per-location and
// possible-type-extension Tier 2 gap-fill passes.

package lint

import (
	"testing"

	"github.com/getoutreach/devbase/v2/internal/graphql/config"
	"gotest.tools/v3/assert"
)

// nonRepeatableDirective is SDL for a directive definition that, per the
// GraphQL spec, may be used at most once per location -- the case
// gapFillDirectivesPerLocation exists to catch.
const nonRepeatableDirective = `directive @foo on OBJECT | SCHEMA`

func TestGapFillDirectivesPerLocationOnSameTypeDefinition(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "schema.graphql", nonRepeatableDirective+`
		type Foo @foo @foo {
			a: String
		}
	`)

	violations, err := Files([]string{path}, nil)
	assert.NilError(t, err)
	assert.Equal(t, len(violations), 1)
	assert.Equal(t, violations[0].Rule, config.RuleUniqueDirectiveNamesPerLocation)
	assert.ErrorContains(t, violations[0].err, `The directive "@foo" can only be used once at this location.`)
}

// TestGapFillDirectivesPerLocationSplitAcrossExtension confirms the gap
// gqlparser's own comment explicitly declines to close: a non-repeatable
// directive used once on a type's base definition and once on a separate
// extension of it. @graphql-eslint (via graphql-js's
// UniqueDirectivesPerLocationRule) treats a type's base definition and all
// of its extensions as one location, so this is still a violation even
// though gqlparser accepts it.
func TestGapFillDirectivesPerLocationSplitAcrossExtension(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "schema.graphql", nonRepeatableDirective+`
		type Foo @foo {
			a: String
		}
		extend type Foo @foo {
			b: String
		}
	`)

	violations, err := Files([]string{path}, nil)
	assert.NilError(t, err)
	assert.Equal(t, len(violations), 1)
	assert.Equal(t, violations[0].Rule, config.RuleUniqueDirectiveNamesPerLocation)
}

func TestGapFillDirectivesPerLocationOnSchemaSplitAcrossExtension(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "schema.graphql", nonRepeatableDirective+`
		schema @foo {
			query: Query
		}
		extend schema @foo
		type Query { a: String }
	`)

	violations, err := Files([]string{path}, nil)
	assert.NilError(t, err)
	assert.Equal(t, len(violations), 1)
	assert.Equal(t, violations[0].Rule, config.RuleUniqueDirectiveNamesPerLocation)
}

func TestGapFillDirectivesPerLocationRepeatableDirectiveNeverFlagged(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "schema.graphql", `
		directive @foo repeatable on OBJECT
		type Foo @foo @foo {
			a: String
		}
	`)

	violations, err := Files([]string{path}, nil)
	assert.NilError(t, err)
	assert.Equal(t, len(violations), 0)
}

// TestGapFillDirectivesPerLocationDifferentTypesEachOnce confirms two
// different types each using the same non-repeatable directive once are
// two distinct locations, not a violation.
func TestGapFillDirectivesPerLocationDifferentTypesEachOnce(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "schema.graphql", nonRepeatableDirective+`
		type Foo @foo {
			a: String
		}
		type Bar @foo {
			a: String
		}
	`)

	violations, err := Files([]string{path}, nil)
	assert.NilError(t, err)
	assert.Equal(t, len(violations), 0)
}

// TestGapFillDirectivesPerLocationFederationDirectivesNeverFalsePositive
// confirms the Apollo Federation directives federation.go synthesizes as
// repeatable (for example @key) never trip this pass, even when a schema
// legitimately keys a type on more than one field set.
func TestGapFillDirectivesPerLocationFederationDirectivesNeverFalsePositive(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "schema.graphql", `
		extend schema @link(url: "https://specs.apollo.dev/federation/v2.3", import: ["@key"])

		type Widget @key(fields: "id") @key(fields: "sku") {
			id: ID!
			sku: String!
		}
	`)

	violations, err := Files([]string{path}, &config.Lint{Federation: "v2.3"})
	assert.NilError(t, err)
	assert.Equal(t, len(violations), 0)
}

func TestGapFillPossibleTypeExtensionOfUndefinedTypeFails(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "schema.graphql", `
		type Query { a: String }
		extend type Missing {
			b: String
		}
	`)

	violations, err := Files([]string{path}, nil)
	assert.NilError(t, err)
	assert.Equal(t, len(violations), 1)
	assert.Equal(t, violations[0].Rule, config.RulePossibleTypeExtension)
	assert.ErrorContains(t, violations[0].err, `Cannot extend type "Missing" because it is not defined.`)
}

func TestGapFillPossibleTypeExtensionOfDefinedTypePasses(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "schema.graphql", `
		type Query { a: String }
		type Foo { a: String }
		extend type Foo {
			b: String
		}
	`)

	violations, err := Files([]string{path}, nil)
	assert.NilError(t, err)
	assert.Equal(t, len(violations), 0)
}

// TestGapFillPossibleTypeExtensionReportsEveryExtensionOfAnUndefinedType
// confirms every extension of the same never-defined type is its own
// violation, matching graphql-js's PossibleTypeExtensionsRule, which
// visits and reports on each extension node independently.
func TestGapFillPossibleTypeExtensionReportsEveryExtensionOfAnUndefinedType(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "schema.graphql", `
		type Query { a: String }
		extend type Missing {
			b: String
		}
		extend type Missing {
			c: String
		}
	`)

	violations, err := Files([]string{path}, nil)
	assert.NilError(t, err)
	assert.Equal(t, len(violations), 2)
	for _, v := range violations {
		assert.Equal(t, v.Rule, config.RulePossibleTypeExtension)
	}
}

// TestGapFillPossibleTypeExtensionOfScalarsPreludeTypePasses confirms a
// scalar scripts/devbase.yaml declares via graphql.lint.scalars counts as
// defined, since federation.go's scalars prelude synthesizes a real
// `scalar X` declaration for it, not merely a reference.
func TestGapFillPossibleTypeExtensionOfScalarsPreludeTypePasses(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "schema.graphql", `
		directive @foo on SCALAR
		type Query { a: JSON }
		extend scalar JSON @foo
	`)

	violations, err := Files([]string{path}, &config.Lint{Scalars: []string{"JSON"}})
	assert.NilError(t, err)
	assert.Equal(t, len(violations), 0)
}
