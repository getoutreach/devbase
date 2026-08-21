// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Empirical coverage tests for the Tier 1 spec validation
// rules from RFC 0006, run against the pinned gqlparser/v2 version.

package spec

import (
	"testing"

	"github.com/vektah/gqlparser/v2"
	"github.com/vektah/gqlparser/v2/ast"
	"gotest.tools/v3/assert"
)

// tier1Case is a single Tier 1 rule's minimal SDL fixture: sdl violates
// exactly the rule under test, and wantErrSubstring is a substring of the
// gqlparser.LoadSchema error confirming the fixture failed for that rule,
// not some other validation.
type tier1Case struct {
	rule             string
	sdl              string
	wantErrSubstring string
}

// TestGqlparserCoverageOfTier1Rules empirically confirms that
// gqlparser.LoadSchema surfaces a parse error for each of the 9 Tier 1
// spec validation rules enumerated in RFC 0006, using the gqlparser/v2
// version pinned in go.mod. Each fixture is written to violate exactly
// one rule.
func TestGqlparserCoverageOfTier1Rules(t *testing.T) {
	cases := []tier1Case{
		{
			rule: "unique-directive-names",
			sdl: `
				directive @foo on FIELD_DEFINITION
				directive @foo on FIELD_DEFINITION
			`,
			wantErrSubstring: "Cannot redeclare directive foo.",
		},
		{
			rule: "unique-field-definition-names",
			sdl: `
				type Foo {
					bar: String
					bar: Int
				}
			`,
			wantErrSubstring: "Field Foo.bar can only be defined once.",
		},
		{
			// The cross-block case: gqlparser rejects a second schema { }
			// block outright, which also catches a duplicate root
			// operation type declared across two blocks. See
			// TestUniqueOperationTypesWithinSingleSchemaBlockGap below for
			// the within-block gap this rule does not catch.
			rule: "unique-operation-types",
			sdl: `
				schema { query: Query }
				schema { query: OtherQuery }
				type Query { a: String }
				type OtherQuery { a: String }
			`,
			wantErrSubstring: "Cannot have multiple schema entry points",
		},
		{
			rule: "unique-type-names",
			sdl: `
				type Foo { a: String }
				type Foo { b: String }
			`,
			wantErrSubstring: "Cannot redeclare type Foo.",
		},
		{
			rule: "known-argument-names",
			sdl: `
				directive @foo(arg: String) on FIELD_DEFINITION
				type Foo {
					bar: String @foo(argX: "test")
				}
			`,
			wantErrSubstring: "Undefined argument argX for directive foo.",
		},
		{
			rule: "known-directives",
			sdl: `
				type Foo {
					bar: String @undefinedDirective
				}
			`,
			wantErrSubstring: "Undefined directive undefinedDirective.",
		},
		{
			rule: "known-type-names",
			sdl: `
				type Foo {
					bar: UndefinedType
				}
			`,
			wantErrSubstring: "Undefined type UndefinedType.",
		},
		{
			rule: "provided-required-arguments",
			sdl: `
				directive @foo(arg: String!) on FIELD_DEFINITION
				type Foo {
					bar: String @foo
				}
			`,
			wantErrSubstring: "Argument arg for directive foo cannot be null.",
		},
		{
			// Distinct from the unique-operation-types fixture above: the
			// two schema blocks here declare disjoint root operations, so
			// only "one schema definition allowed" -- not a duplicate root
			// operation type -- explains the error.
			rule: "lone-schema-definition",
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
			_, err := gqlparser.LoadSchema(&ast.Source{Input: c.sdl})
			assert.ErrorContains(t, err, c.wantErrSubstring)
		})
	}
}

// TestUniqueOperationTypesWithinSingleSchemaBlockGap documents the
// unique-operation-types gap from RFC 0006: gqlparser only rejects
// multiple schema { } blocks. Two "query:" entries inside a single block
// parse without error, and gqlparser silently keeps the last one. A Tier 2
// gap-fill pass is required to flag this case (matching
// @graphql-eslint's behavior), since Tier 1 alone does not catch it.
func TestUniqueOperationTypesWithinSingleSchemaBlockGap(t *testing.T) {
	sdl := `
		schema {
			query: Query
			query: OtherQuery
		}
		type Query { a: String }
		type OtherQuery { a: String }
	`

	schema, err := gqlparser.LoadSchema(&ast.Source{Input: sdl})
	assert.NilError(t, err)
	assert.Equal(t, schema.Query.Name, "OtherQuery")
}
