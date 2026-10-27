// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Tests for "# eslint <rule>: <config>" inline configuration
// comments.

// Expected results below were checked against
// @graphql-eslint/eslint-plugin 3.20.1 running under ESLint 8: such a
// comment applies to its whole file wherever it appears, replaces the
// rule's configured options entirely rather than merging with them, and
// the last of several conflicting comments wins.

package lint

import (
	"fmt"
	"testing"

	"github.com/getoutreach/devbase/v2/internal/graphql/config"
	"gotest.tools/v3/assert"
)

// namingOnlyCfg enables naming-convention the way the stencil-graphql
// template does, and nothing else.
func namingOnlyCfg() *config.Lint {
	return &config.Lint{Rules: map[string]config.Rule{
		config.RuleNamingConvention: {
			Severity: config.SeverityError,
			Options: map[string]any{
				"types":               "PascalCase",
				"FieldDefinition":     "camelCase",
				"EnumValueDefinition": "UPPER_CASE",
			},
		},
	}}
}

func TestEslintConfigComments(t *testing.T) {
	enumValue := func(v string) string { return fmt.Sprintf(namingFor, v) }

	cases := []struct {
		name string
		sdl  string
		want []string
	}{
		{
			"options are replaced for the whole file, wherever the comment is",
			`enum E {
				lower
			}
			# eslint @graphql-eslint/naming-convention: ["error", { FieldDefinition: "camelCase" }]
			type t {
				Bad_field: String
			}`,
			[]string{`naming-convention Field "Bad_field" should be in camelCase format`},
		},
		{
			"off disables the rule for the file",
			`# eslint @graphql-eslint/naming-convention: "off"
			enum E {
				lower
			}`,
			nil,
		},
		{
			"a numeric severity of 0 disables the rule",
			`# eslint @graphql-eslint/naming-convention: 0
			enum E {
				lower
			}`,
			nil,
		},
		{
			"a severity alone replaces the options with none",
			`# eslint @graphql-eslint/naming-convention: "warn"
			enum E {
				lower
			}`,
			nil,
		},
		{
			"the last of several comments wins",
			`# eslint @graphql-eslint/naming-convention: ["error", { EnumValueDefinition: "UPPER_CASE" }]
			enum E {
				lower
				UPPER
			}
			# eslint @graphql-eslint/naming-convention: ["error", { EnumValueDefinition: "camelCase" }]`,
			[]string{`naming-convention Enumeration value "UPPER" should be in camelCase format`},
		},
		{
			"one comment configures several rules, including one the config leaves off",
			`# eslint @graphql-eslint/naming-convention: "off", @graphql-eslint/require-description: ["error", { EnumValueDefinition: true }]
			enum E {
				lower
			}`,
			[]string{fmt.Sprintf(descFor, "lower")},
		},
		{
			"a bare devbase rule name is accepted",
			`# eslint naming-convention: "off"
			enum E {
				lower
			}`,
			nil,
		},
		{
			"a Tier 1 rule cannot be configured",
			`# eslint @graphql-eslint/known-type-names: "off"
			type Foo { a: Missing }`,
			[]string{"known-type-names Undefined type Missing."},
		},
		{
			"a comment that names no known rule is ignored",
			`# eslint is great: really
			enum E {
				lower
			}`,
			[]string{enumValue("lower")},
		},
		{
			"text inside a block string is not a comment",
			`"""
			# eslint @graphql-eslint/naming-convention: "off"
			"""
			enum E {
				lower
			}`,
			[]string{enumValue("lower")},
		},
		{
			"a reason after -- is ignored",
			`# eslint @graphql-eslint/naming-convention: "off" -- Reason: Existing enum.
			enum E {
				lower
			}`,
			nil,
		},
		{
			"a numeric severity of 1 in a list keeps the options",
			`# eslint @graphql-eslint/naming-convention: [1, { EnumValueDefinition: "camelCase" }]
			enum E {
				UPPER
			}`,
			[]string{`naming-convention Enumeration value "UPPER" should be in camelCase format`},
		},
		{
			"a numeric severity of 2 in a list keeps the options",
			`# eslint @graphql-eslint/naming-convention: [2, { EnumValueDefinition: "camelCase" }]
			enum E {
				UPPER
			}`,
			[]string{`naming-convention Enumeration value "UPPER" should be in camelCase format`},
		},
		{
			"a one-element list replaces the options with none",
			`# eslint @graphql-eslint/naming-convention: ["error"]
			enum E {
				lower
			}`,
			nil,
		},
		{
			"an eslint-disable-next-line still applies under a configuration comment",
			`# eslint @graphql-eslint/naming-convention: ["error", { EnumValueDefinition: "camelCase" }]
			enum E {
				# eslint-disable-next-line @graphql-eslint/naming-convention
				UPPER
				SHOUT
			}`,
			[]string{`naming-convention Enumeration value "SHOUT" should be in camelCase format`},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := ruleMessages(t, c.sdl, namingOnlyCfg())
			if c.want == nil {
				assert.Equal(t, len(got), 0, "got %v", got)
				return
			}
			assert.DeepEqual(t, got, c.want)
		})
	}
}

// TestEslintConfigCommentIsPerFile confirms a comment in one file does not
// change the rule's configuration in another file.
func TestEslintConfigCommentIsPerFile(t *testing.T) {
	dir := t.TempDir()
	a := writeFile(t, dir, "a.graphql", "# eslint @graphql-eslint/naming-convention: \"off\"\nenum A {\n\tx\n}\n")
	b := writeFile(t, dir, "b.graphql", "enum B {\n\ty\n}\n")

	violations, err := Files([]string{a, b}, namingOnlyCfg())
	assert.NilError(t, err)
	assert.Equal(t, len(violations), 1)
	assert.Equal(t, violations[0].File(), b)
}

// TestEslintConfigCommentEnablesRuleWithNoConfig confirms a comment can
// turn on a rule when scripts/devbase.yaml configures nothing at all.
func TestEslintConfigCommentEnablesRuleWithNoConfig(t *testing.T) {
	got := ruleMessages(t, `# eslint @graphql-eslint/naming-convention: ["error", { EnumValueDefinition: "UPPER_CASE" }]
		enum E {
			lower
		}`, nil)

	assert.DeepEqual(t, got, []string{fmt.Sprintf(namingFor, "lower")})
}

// TestEslintConfigCommentsApplyPerConfiguration confirms files that share
// an override, and files that declare a different one, each get results
// from their own configuration.
func TestEslintConfigCommentsApplyPerConfiguration(t *testing.T) {
	dir := t.TempDir()
	const prefix = `# eslint @graphql-eslint/naming-convention: ["error", { EnumValueDefinition: `
	a := writeFile(t, dir, "a.graphql", prefix+"\"camelCase\" }]\nenum A {\n\tX\n}\n")
	b := writeFile(t, dir, "b.graphql", prefix+"\"camelCase\" }]\nenum B {\n\tY\n}\n")
	c := writeFile(t, dir, "c.graphql", prefix+"\"snake_case\" }]\nenum C {\n\tZ\n}\n")

	violations, err := Files([]string{a, b, c}, namingOnlyCfg())
	assert.NilError(t, err)

	got := make(map[string]string, len(violations))
	for _, v := range violations {
		got[v.File()] = v.Message()
	}
	assert.DeepEqual(t, got, map[string]string{
		a: `Enumeration value "X" should be in camelCase format`,
		b: `Enumeration value "Y" should be in camelCase format`,
		c: `Enumeration value "Z" should be in snake_case format`,
	})
}

func TestSplitTopLevel(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []string
	}{
		{"empty", ``, []string{""}},
		{"plain entries", `a, b`, []string{"a", " b"}},
		{"commas inside brackets and braces", `a: [1, 2], b: {c: 3, d: 4}`, []string{"a: [1, 2]", " b: {c: 3, d: 4}"}},
		{"comma inside double quotes", `a: "x, y", b`, []string{`a: "x, y"`, " b"}},
		{"comma inside single quotes", `a: 'x, y', b`, []string{`a: 'x, y'`, " b"}},
		{"escaped quote inside double quotes", `a: "say \"hi, there\"", b`, []string{`a: "say \"hi, there\""`, " b"}},
		{"bracket inside a quoted string", `a: "[", b`, []string{`a: "["`, " b"}},
		{"unclosed bracket keeps the rest together", `a: [1, 2`, []string{"a: [1, 2"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.DeepEqual(t, splitTopLevel(c.in), c.want)
		})
	}
}
