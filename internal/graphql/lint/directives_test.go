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

// enableRule returns a Lint config with rule set to SeverityError, the
// minimum scripts/devbase.yaml entry needed to turn a Tier 2 or Tier 3
// rule on: neither runs by default (config.Lint.Enabled), matching
// @graphql-eslint's own behavior of a rule staying inert until a config
// opts into it.
func enableRule(rule string) *config.Lint {
	return &config.Lint{Rules: map[string]config.Rule{rule: {Severity: config.SeverityError}}}
}

func TestGapFillDirectivesPerLocationOnSameTypeDefinition(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "schema.graphql", nonRepeatableDirective+`
		type Foo @foo @foo {
			a: String
		}
	`)

	violations, err := Files([]string{path}, enableRule(config.RuleUniqueDirectiveNamesPerLocation))
	assert.NilError(t, err)
	assert.Equal(t, len(violations), 1)
	assert.Equal(t, violations[0].Rule, config.RuleUniqueDirectiveNamesPerLocation)
	assert.ErrorContains(t, violations[0].err, `The directive "@foo" can only be used once at this location.`)
}

// TestGapFillDirectivesPerLocationDisabledByDefault confirms the same
// violation as TestGapFillDirectivesPerLocationOnSameTypeDefinition is not
// reported when scripts/devbase.yaml never enables the rule -- including
// when cfg is nil, i.e. no config file was found at all.
func TestGapFillDirectivesPerLocationDisabledByDefault(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "schema.graphql", nonRepeatableDirective+`
		type Foo @foo @foo {
			a: String
		}
	`)

	violations, err := Files([]string{path}, nil)
	assert.NilError(t, err)
	assert.Equal(t, len(violations), 0)

	violations, err = Files([]string{path}, &config.Lint{
		Rules: map[string]config.Rule{config.RuleUniqueDirectiveNamesPerLocation: {Severity: config.SeverityOff}},
	})
	assert.NilError(t, err)
	assert.Equal(t, len(violations), 0)
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

	violations, err := Files([]string{path}, enableRule(config.RuleUniqueDirectiveNamesPerLocation))
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

	violations, err := Files([]string{path}, enableRule(config.RuleUniqueDirectiveNamesPerLocation))
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

	violations, err := Files([]string{path}, enableRule(config.RuleUniqueDirectiveNamesPerLocation))
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

	violations, err := Files([]string{path}, enableRule(config.RuleUniqueDirectiveNamesPerLocation))
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

	cfg := enableRule(config.RuleUniqueDirectiveNamesPerLocation)
	cfg.Federation = "v2.3"
	violations, err := Files([]string{path}, cfg)
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

	violations, err := Files([]string{path}, enableRule(config.RulePossibleTypeExtension))
	assert.NilError(t, err)
	assert.Equal(t, len(violations), 1)
	assert.Equal(t, violations[0].Rule, config.RulePossibleTypeExtension)
	assert.ErrorContains(t, violations[0].err, `Cannot extend type "Missing" because it is not defined.`)
}

// TestGapFillPossibleTypeExtensionDisabledByDefault confirms the same
// violation as TestGapFillPossibleTypeExtensionOfUndefinedTypeFails is not
// reported when scripts/devbase.yaml never enables the rule.
func TestGapFillPossibleTypeExtensionDisabledByDefault(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "schema.graphql", `
		type Query { a: String }
		extend type Missing {
			b: String
		}
	`)

	violations, err := Files([]string{path}, nil)
	assert.NilError(t, err)
	assert.Equal(t, len(violations), 0)
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

	violations, err := Files([]string{path}, enableRule(config.RulePossibleTypeExtension))
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

	violations, err := Files([]string{path}, enableRule(config.RulePossibleTypeExtension))
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

	cfg := enableRule(config.RulePossibleTypeExtension)
	cfg.Scalars = []string{"JSON"}
	violations, err := Files([]string{path}, cfg)
	assert.NilError(t, err)
	assert.Equal(t, len(violations), 0)
}
