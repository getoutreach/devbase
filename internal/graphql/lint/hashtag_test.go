// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Tests for the no-hashtag-description Tier 3 rule.

// Cases below are ported from @graphql-eslint/eslint-plugin's own test
// suite (packages/plugin/src/rules/no-hashtag-description/index.test.ts,
// graphql-hive/graphql-eslint@master as of 2026-08-22), with two
// adaptations. Cases exercising an OperationDefinition or
// FragmentDefinition document are dropped, since devbase graphql lint
// has no notion of either: it only ever parses standalone *.graphql
// schema files (see lint.go's Files), never query/mutation/subscription/
// fragment documents. And the "# eslint-disable-next-line" case is
// inverted from valid to invalid: devbase has no inline-suppression
// comment syntax anywhere in its lint package, so to it that line is
// just an ordinary attached comment like any other; see
// TestNoHashtagDescriptionInvalidCases' last case.

package lint

import (
	"testing"

	"github.com/getoutreach/devbase/v2/internal/graphql/config"
	"gotest.tools/v3/assert"
)

func TestNoHashtagDescriptionValidCases(t *testing.T) {
	cases := []struct {
		name string
		sdl  string
	}{
		{
			"inline description instead of a hashtag comment",
			`" Good "
			type Query {
				foo: String
			}`,
		},
		{
			"leading and trailing comments separated from the type by a blank line",
			`# Good

			type Query {
				foo: String
			}
			# Good`,
		},
		{
			"multiline leading comment block separated from the type by a blank line",
			`# multiline
			# multiline
			# multiline

			type Query {
				foo: String
			}`,
		},
		{
			"same-line trailing comments only",
			`type Query { # Good
				foo: String # Good
			} # Good`,
		},
		{
			"comment before a field, separated from it by a blank line",
			`type Query {
				# Good

				foo: ID
			}`,
		},
		{
			"comment between two fields, separated from the second by a blank line",
			`type Query {
				foo: ID
				# Good

				bar: ID
			}`,
		},
		{
			"comment before an argument, separated from it by a blank line",
			`type User { id: ID }
			type Query {
				user(
					# Good

					id: Int
				): User
			}`,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			path := writeFile(t, dir, "schema.graphql", c.sdl)

			violations, err := Files([]string{path}, enableRule(config.RuleNoHashtagDescription))
			assert.NilError(t, err)
			assert.Equal(t, len(violations), 0)
		})
	}
}

func TestNoHashtagDescriptionInvalidCases(t *testing.T) {
	cases := []singleViolationCase{
		{
			"comment immediately before the type",
			`# Bad
			type Query {
				foo: String
			}`,
		},
		{
			"multiline comment block immediately before the type",
			`# multiline
			# multiline
			type Query {
				foo: String
			}`,
		},
		{
			"comment immediately before a field",
			`type Query {
				# Bad
				foo: String
			}`,
		},
		{
			"comment between two fields, immediately before the second",
			`type Query {
				bar: ID
				# Bad
				foo: ID
				# Good
			}`,
		},
		{
			"comment immediately before an argument",
			`type User { id: ID }
			type Query {
				user(
					# Bad
					id: Int!
				): User
			}`,
		},
		{
			"a comment shaped like an eslint directive is still an ordinary comment to devbase",
			`# eslint-disable-next-line
			type Query {
				foo: String
			}`,
		},
	}
	runSingleViolationCases(t, config.RuleNoHashtagDescription, "Unexpected GraphQL description as hashtag", cases)
}

// TestNoHashtagDescriptionDisabledByDefault confirms the same violation as
// TestNoHashtagDescriptionInvalidCases' first case is not reported when
// scripts/devbase.yaml never enables the rule, including when cfg is
// nil, i.e. no config file was found at all.
func TestNoHashtagDescriptionDisabledByDefault(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "schema.graphql", `# Bad
	type Query {
		foo: String
	}`)

	violations, err := Files([]string{path}, nil)
	assert.NilError(t, err)
	assert.Equal(t, len(violations), 0)
}
