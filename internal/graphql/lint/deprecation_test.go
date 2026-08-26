// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Tests for the require-deprecation-reason and
// require-deprecation-date Tier 3 rules.

// Cases below are ported from @graphql-eslint/eslint-plugin's own test
// suite (packages/plugin/src/rules/require-deprecation-reason/index.test.ts
// and packages/plugin/src/rules/require-deprecation-date/index.test.ts,
// graphql-hive/graphql-eslint@master as of 2026-08-22), adapted to
// devbase's stricter validation in two ways. First, each case is split
// into its own minimal, independently-valid SDL fixture rather than
// upstream's one large combined document: Files stops at the first
// Tier 1 violation it finds (see lint.go's own doc comment), so a
// document mixing multiple unrelated problems can only ever surface
// one of them here. Second, cases relying on an undefined scalar type
// (`Number`), an undefined directive (`@authorized`), or an
// @deprecated usage at a location only the builtin definition's
// Locations allow (`OBJECT`, `SCALAR`) are dropped entirely: gqlparser
// validates real schemas, unlike @graphql-eslint's AST-only rule
// tests, so none of those could ever reach Tier 3 in devbase; see
// deprecation.go's own doc comment on the built-in @deprecated
// signature.

package lint

import (
	"testing"
	"time"

	"github.com/getoutreach/devbase/v2/internal/graphql/config"
	"gotest.tools/v3/assert"
)

func TestRequireDeprecationReasonValidCases(t *testing.T) {
	cases := []struct {
		name string
		sdl  string
	}{
		{"no @deprecated usage", `type MyType { name: String }`},
		{"field with a reason", `type MyType { name: String @deprecated(reason: "no longer relevant, use fullName") }`},
		// @graphql-eslint's own rule stringifies the reason argument's
		// value before checking it for blankness, so a non-string
		// literal reason still counts as present.
		{"enum value with a numeric reason", `enum TestEnum { item1 @deprecated(reason: 0) item2 }`},
		{"interface field with a float reason", `interface TestInterface { field1: Float @deprecated(reason: 1.5) }`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			path := writeFile(t, dir, "schema.graphql", c.sdl)

			violations, err := Files([]string{path}, enableRule(config.RuleRequireDeprecationReason))
			assert.NilError(t, err)
			assert.Equal(t, len(violations), 0)
		})
	}
}

func TestRequireDeprecationReasonInvalidCases(t *testing.T) {
	cases := []singleViolationCase{
		{"field with no reason", `type A { deprecatedWithoutReason: String @deprecated }`},
		{"enum value with no reason", `enum TestEnum { item1 @deprecated }`},
		{"field with an empty reason", `interface TestInterface { item1: String @deprecated(reason: "") }`},
		{"field with a whitespace-only reason", `type B { item1: String @deprecated(reason: "  ") }`},
		{"input field with no reason", `input MyInput { foo: String @deprecated }`},
		{"directive argument with no reason", `directive @foo(bar: String @deprecated) on FIELD`},
	}
	runSingleViolationCases(t, config.RuleRequireDeprecationReason, "Deprecation reason is required for", cases)
}

// TestRequireDeprecationReasonDisabledByDefault confirms the same
// violation as the first TestRequireDeprecationReasonInvalidCases case is
// not reported when scripts/devbase.yaml never enables the rule,
// including when cfg is nil, i.e. no config file was found at all.
func TestRequireDeprecationReasonDisabledByDefault(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "schema.graphql", `type A { deprecatedWithoutReason: String @deprecated }`)

	violations, err := Files([]string{path}, nil)
	assert.NilError(t, err)
	assert.Equal(t, len(violations), 0)
}

// dateString formats when as require-deprecation-date's "DD/MM/YYYY"
// argument value, computed relative to time.Now() so these tests never
// go stale the way a hardcoded calendar date eventually would.
func dateString(when time.Time) string {
	return when.Format(deletionDateLayout)
}

// redeclareDeprecated returns SDL that redeclares the built-in
// @deprecated directive with an extra argName argument. A repository
// wanting require-deprecation-date must do the same in its own SDL,
// since gqlparser rejects an argument on a @deprecated usage unless the
// schema's own @deprecated declaration names it too (see deprecation.go's
// doc comment on the built-in directive's silent redeclaration).
func redeclareDeprecated(argName string) string {
	return `directive @deprecated(
		reason: String = "No longer supported"
		` + argName + `: String
	) on FIELD_DEFINITION | ARGUMENT_DEFINITION | INPUT_FIELD_DEFINITION | ENUM_VALUE
	`
}

func TestRequireDeprecationDateValidCases(t *testing.T) {
	tomorrow := dateString(time.Now().AddDate(0, 0, 1))
	farFuture := dateString(time.Now().AddDate(5, 0, 0))

	cases := []struct {
		name string
		sdl  string
		opts map[string]any
	}{
		{"no @deprecated usage", `type User { firstName: String }`, nil},
		{
			"future deletion date",
			redeclareDeprecated("deletionDate") + `type Widget { old: String @deprecated(deletionDate: "` + tomorrow + `") }`,
			nil,
		},
		{
			"future deletion date under a custom argument name",
			redeclareDeprecated("untilDate") + `type Widget { old: String @deprecated(untilDate: "` + tomorrow + `") }`,
			map[string]any{"argumentName": "untilDate"},
		},
		{
			"field with both a reason and a far-future deletion date",
			redeclareDeprecated("deletionDate") + `type User {
				firstname: String @deprecated(reason: "Use 'firstName' instead", deletionDate: "` + farFuture + `")
				firstName: String
			}`,
			nil,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			path := writeFile(t, dir, "schema.graphql", c.sdl)

			violations, err := Files([]string{path}, enableRuleWithOptions(config.RuleRequireDeprecationDate, c.opts))
			assert.NilError(t, err)
			assert.Equal(t, len(violations), 0)
		})
	}
}

func TestRequireDeprecationDateInvalidCases(t *testing.T) {
	yesterday := dateString(time.Now().AddDate(0, 0, -1))

	cases := []struct {
		name            string
		sdl             string
		opts            map[string]any
		wantErrContains string
	}{
		{
			"deletion date already passed",
			redeclareDeprecated("deletionDate") + `type Widget { old: String @deprecated(deletionDate: "` + yesterday + `") }`,
			nil,
			"can be removed",
		},
		{
			"deletion date already passed under a custom argument name",
			redeclareDeprecated("untilDate") + `type Widget { old: String @deprecated(untilDate: "` + yesterday + `") }`,
			map[string]any{"argumentName": "untilDate"},
			"can be removed",
		},
		{
			"malformed deletion date",
			redeclareDeprecated("deletionDate") + `type Widget { old: String @deprecated(deletionDate: "bad") }`,
			nil,
			`must be in format "DD/MM/YYYY"`,
		},
		{
			"deletion date naming a day that does not exist",
			redeclareDeprecated("deletionDate") + `type Widget { old: String @deprecated(deletionDate: "32/08/2021") }`,
			nil,
			`Invalid "32/08/2021" deletion date`,
		},
		{
			"missing deletion date",
			`type Old { oldField: ID @deprecated }`,
			nil,
			`Directive "@deprecated" must have a deletion date`,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			path := writeFile(t, dir, "schema.graphql", c.sdl)

			violations, err := Files([]string{path}, enableRuleWithOptions(config.RuleRequireDeprecationDate, c.opts))
			assert.NilError(t, err)
			assert.Equal(t, len(violations), 1)
			assert.Equal(t, violations[0].Rule, config.RuleRequireDeprecationDate)
			assert.ErrorContains(t, violations[0].err, c.wantErrContains)
		})
	}
}

// TestRequireDeprecationDateDisabledByDefault confirms the same violation
// as TestRequireDeprecationDateInvalidCases' "missing deletion date" case
// is not reported when scripts/devbase.yaml never enables the rule,
// including when cfg is nil, i.e. no config file was found at all.
func TestRequireDeprecationDateDisabledByDefault(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "schema.graphql", `type Old { oldField: ID @deprecated }`)

	violations, err := Files([]string{path}, nil)
	assert.NilError(t, err)
	assert.Equal(t, len(violations), 0)
}
