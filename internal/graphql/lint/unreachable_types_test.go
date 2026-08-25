// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Tests for the no-unreachable-types Tier 3 rule.

// Cases below are ported from @graphql-eslint/eslint-plugin's own test
// suite (packages/plugin/tests/no-unreachable-types.spec.ts,
// @graphql-eslint/eslint-plugin@3.13.1, the version this port targets).
// @graphql-eslint's own rule tester never
// runs full graphql-js schema validation, so a few of its fixtures are
// not spec-valid SDL on their own terms: a scalar union member, a
// fieldless object type, or an object type omitting an interface its
// own interface itself implements. gqlparser validates all of these as
// part of devbase's Tier 1 pass, ahead of Tier 3 ever running, so those
// fixtures are adjusted here (a spec-valid stand-in type or field) to
// reach this rule at all, while still exercising the same reachability
// behavior.

package lint

import (
	"testing"

	"github.com/getoutreach/devbase/v2/internal/graphql/config"
	"gotest.tools/v3/assert"
)

func TestNoUnreachableTypesValidCases(t *testing.T) {
	cases := []struct {
		name string
		sdl  string
	}{
		{
			"union member reachable through a field",
			`
				type A { a: String }
				type B { b: String }
				union Response = A | B
				type Query { foo: Response }
			`,
		},
		{
			"object type reachable through a field",
			`
				type Query { me: User }
				type User { id: ID name: String }
			`,
		},
		{
			"interface implementation reachable through its interface",
			`
				type Query { me: User }
				interface Address { city: String }
				type User implements Address { city: String }
			`,
		},
		{
			"scalar reachable through a field",
			`
				scalar DateTime
				type Query { now: DateTime }
			`,
		},
		{
			"enum reachable through a field",
			`
				enum Role { ADMIN USER }
				type Query { role: Role }
			`,
		},
		{
			"input object reachable through an argument",
			`
				input UserInput { id: ID }
				type Query { user(input: UserInput!): Boolean }
			`,
		},
		{
			"directive reachable through a usage",
			`
				directive @auth(role: [Role!]!) on FIELD_DEFINITION
				enum Role { ADMIN USER }
				type Query { user: ID @auth(role: [ADMIN]) }
			`,
		},
		{
			"root types reachable through an explicit schema definition",
			`
				type RootQuery { ping: Boolean }
				type RootMutation { ping: Boolean }
				type RootSubscription { ping: Boolean }
				schema { query: RootQuery mutation: RootMutation subscription: RootSubscription }
			`,
		},
		{
			"reaching an interface reaches its transitive implementors",
			`
				interface User { id: ID! }
				interface Manager implements User { id: ID! }
				type TopManager implements Manager & User { id: ID! name: String }
				type Query { me: User }
			`,
		},
		{
			"directive on schema",
			`
				type Query { ping: Boolean }
				schema @good { query: Query }
				directive @good on SCHEMA
			`,
		},
		{
			"directives with request locations are reachable on their own",
			`
				directive @q on QUERY
				directive @w on MUTATION
				directive @e on SUBSCRIPTION
				directive @r on FIELD
				directive @t on FRAGMENT_DEFINITION
				directive @y on FRAGMENT_SPREAD
				directive @u on INLINE_FRAGMENT
				directive @i on VARIABLE_DEFINITION
				type Query { ping: Boolean }
			`,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			path := writeFile(t, dir, "schema.graphql", c.sdl)

			violations, err := Files([]string{path}, enableRule(config.RuleNoUnreachableTypes))
			assert.NilError(t, err)
			assert.Equal(t, len(violations), 0)
		})
	}
}

func TestNoUnreachableTypesInvalidCases(t *testing.T) {
	cases := []struct {
		name         string
		sdl          string
		wantMessages []string
	}{
		{
			"unreachable interfaces and their unreachable implementor",
			`
				type Query { node(id: ID!): AnotherNode! }
				interface Node { id: ID! }
				interface AnotherNode { createdAt: String }
				interface User implements Node { id: ID! name: String }
				type SuperUser implements User & Node { id: ID! name: String address: String }
			`,
			[]string{
				"Interface type `Node` is unreachable.",
				"Interface type `User` is unreachable.",
				"Object type `SuperUser` is unreachable.",
			},
		},
		{
			"every kind unreachable with no root type referencing any of them",
			`
				scalar DateTime
				enum Role { ADMIN USER }
				directive @auth(role: [String!]!) on FIELD_DEFINITION
				union Union = User
				input UsersFilter { limit: Int }
				interface Address { city: String }
				type User implements Address { city: String }
			`,
			[]string{
				"Scalar type `DateTime` is unreachable.",
				"Enum type `Role` is unreachable.",
				"Directive `auth` is unreachable.",
				"Union type `Union` is unreachable.",
				"Input object type `UsersFilter` is unreachable.",
				"Interface type `Address` is unreachable.",
				"Object type `User` is unreachable.",
			},
		},
		{
			"one unreachable scalar among otherwise-reachable types",
			`
				interface User { id: String }
				type SuperUser implements User { id: String superDetail: SuperDetail }
				type SuperDetail { detail: String }
				type Query { user: User! }
				scalar DateTime
			`,
			[]string{"Scalar type `DateTime` is unreachable."},
		},
		{
			"a base type and its extension are each reported",
			`
				interface User { id: String }
				interface AnotherUser { createdAt: String }
				type SuperUser implements User { id: String }
				extend type SuperUser { detail: String }
				type Query { user: AnotherUser! }
			`,
			[]string{
				"Interface type `User` is unreachable.",
				"Object type `SuperUser` is unreachable.",
				"Object type `SuperUser` is unreachable.",
			},
		},
		{
			"reachable interface chain leaves an unrelated scalar unreachable",
			`
				type Query { node(id: ID!): Node! }
				interface Node { id: ID! }
				interface User implements Node { id: ID! name: String }
				type SuperUser implements User & Node { id: ID! name: String address: String }
				scalar DateTime
			`,
			[]string{"Scalar type `DateTime` is unreachable."},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			path := writeFile(t, dir, "schema.graphql", c.sdl)

			violations, err := Files([]string{path}, enableRule(config.RuleNoUnreachableTypes))
			assert.NilError(t, err)
			assert.Equal(t, len(violations), len(c.wantMessages))
			for i, want := range c.wantMessages {
				assert.Equal(t, violations[i].Rule, config.RuleNoUnreachableTypes)
				assert.ErrorContains(t, violations[i].err, want)
			}
		})
	}
}

// TestNoUnreachableTypesAcrossFiles is a Go-specific edge case: a root
// type in one file reaching a type defined in another is exactly what
// devbase graphql lint's cross-file schema is for (see lint.go's Files),
// so reachability must follow a reference across a file boundary the
// same as within one file. @graphql-eslint cannot exercise this itself:
// each of its own tests lints one schema string as a single file.
func TestNoUnreachableTypesAcrossFiles(t *testing.T) {
	dir := t.TempDir()
	query := writeFile(t, dir, "query.graphql", `type Query { me: User }`)
	user := writeFile(t, dir, "user.graphql", `type User { id: ID! } scalar Unused`)

	violations, err := Files([]string{query, user}, enableRule(config.RuleNoUnreachableTypes))
	assert.NilError(t, err)
	assert.Equal(t, len(violations), 1)
	assert.Equal(t, violations[0].Rule, config.RuleNoUnreachableTypes)
	assert.ErrorContains(t, violations[0].err, "Scalar type `Unused` is unreachable.")
}
