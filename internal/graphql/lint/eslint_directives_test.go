// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Tests for "# eslint-disable" style suppression comments.

package lint

import (
	"fmt"
	"slices"
	"testing"

	"github.com/getoutreach/devbase/v2/internal/graphql/config"
	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/gqlerror"
	"gotest.tools/v3/assert"
)

// namingAndDescriptionCfg enables naming-convention (enum values must be
// UPPER_CASE) and require-description (enum values need one), so a
// lowercase enum value without a description trips both.
func namingAndDescriptionCfg() *config.Lint {
	return &config.Lint{Rules: map[string]config.Rule{
		config.RuleNamingConvention: {
			Severity: config.SeverityError,
			Options:  map[string]any{"EnumValueDefinition": "UPPER_CASE"},
		},
		config.RuleRequireDescription: {
			Severity: config.SeverityError,
			Options:  map[string]any{"EnumValueDefinition": true},
		},
	}}
}

const (
	namingFor = `naming-convention Enumeration value %q should be in UPPER_CASE format`
	descFor   = `require-description Description is required for enum value %q in enum "E"`
)

// ruleMessages lints sdl and returns each violation as "<rule> <message>",
// sorted so tests do not depend on the order the rules run in.
func ruleMessages(t *testing.T, sdl string, cfg *config.Lint) []string {
	t.Helper()
	path := writeFile(t, t.TempDir(), "schema.graphql", sdl)
	violations, err := Files([]string{path}, cfg)
	assert.NilError(t, err)
	got := make([]string, 0, len(violations))
	for _, v := range violations {
		got = append(got, v.Rule+" "+v.Message())
	}
	slices.Sort(got)
	return got
}

func TestEslintDirectiveSuppression(t *testing.T) {
	naming := func(v string) string { return fmt.Sprintf(namingFor, v) }
	desc := func(v string) string { return fmt.Sprintf(descFor, v) }

	cases := []struct {
		name string
		sdl  string
		want []string
	}{
		{
			"disable and enable bound a region",
			`enum E {
				# eslint-disable @graphql-eslint/naming-convention
				inside
				# eslint-enable @graphql-eslint/naming-convention
				outside
			}`,
			[]string{naming("outside"), desc("inside"), desc("outside")},
		},
		{
			"disable without enable runs to the end of the file",
			`enum E {
				before
				# eslint-disable @graphql-eslint/naming-convention
				after
			}`,
			[]string{naming("before"), desc("before"), desc("after")},
		},
		{
			"disable-next-line covers only the following line",
			`enum E {
				# eslint-disable-next-line @graphql-eslint/naming-convention
				first
				second
			}`,
			[]string{desc("first"), naming("second"), desc("second")},
		},
		{
			"a directive names only the rules it lists",
			`enum E {
				# eslint-disable-next-line @graphql-eslint/naming-convention
				first
			}`,
			[]string{desc("first")},
		},
		{
			"a bare devbase rule name is accepted",
			`enum E {
				# eslint-disable-next-line naming-convention
				first
			}`,
			[]string{desc("first")},
		},
		{
			"no rule listed covers every rule",
			`enum E {
				# eslint-disable-next-line
				first
			}`,
			nil,
		},
		{
			"a reason after -- is ignored",
			`enum E {
				# eslint-disable-next-line @graphql-eslint/naming-convention -- Reason: Existing enum.
				first
			}`,
			[]string{desc("first")},
		},
		{
			"rules can be comma separated",
			`enum E {
				# eslint-disable-next-line @graphql-eslint/naming-convention, @graphql-eslint/require-description
				first
			}`,
			nil,
		},
		{
			"enable of one rule re-enables only that rule after a disable of all",
			`enum E {
				# eslint-disable
				a
				# eslint-enable @graphql-eslint/naming-convention
				b
			}`,
			[]string{naming("b")},
		},
		{
			"a Tier 1 violation cannot be suppressed",
			`# eslint-disable
			type Foo { a: Missing }`,
			[]string{"known-type-names Undefined type Missing."},
		},
		{
			"text inside a block string is not a directive",
			`enum E {
				"""
				# eslint-disable-next-line @graphql-eslint/naming-convention
				"""
				first
			}`,
			[]string{naming("first")},
		},
		{
			"a comment that only resembles a directive is ignored",
			`enum E {
				# eslint-disabled @graphql-eslint/naming-convention
				first
				# see eslint-disable-next-line naming-convention
				second
			}`,
			[]string{naming("first"), desc("first"), naming("second"), desc("second")},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := ruleMessages(t, c.sdl, namingAndDescriptionCfg())
			if c.want == nil {
				assert.Equal(t, len(got), 0, "got %v", got)
				return
			}
			want := slices.Clone(c.want)
			slices.Sort(want)
			assert.DeepEqual(t, got, want)
		})
	}
}

// TestEslintDirectiveIsPerFile confirms a directive in one file does not
// suppress a violation of the same rule in another file.
func TestEslintDirectiveIsPerFile(t *testing.T) {
	dir := t.TempDir()
	a := writeFile(t, dir, "a.graphql", "enum A {\n\t# eslint-disable\n\tx\n}\n")
	b := writeFile(t, dir, "b.graphql", "enum B {\n\ty\n}\n")

	violations, err := Files([]string{a, b}, namingAndDescriptionCfg())
	assert.NilError(t, err)
	assert.Equal(t, len(violations), 2)
	for _, v := range violations {
		assert.Equal(t, v.File(), b)
	}
}

// TestEslintDirectiveSuppressesTier2Rule confirms a directive silences a
// Tier 2 gap-fill rule too, not only the Tier 3 style rules.
func TestEslintDirectiveSuppressesTier2Rule(t *testing.T) {
	const rule = config.RuleUniqueDirectiveNamesPerLocation
	const declaration = "directive @foo on OBJECT\n"

	flagged := writeFile(t, t.TempDir(), "flagged.graphql", declaration+"type Foo @foo @foo { a: String }\n")
	violations, err := Files([]string{flagged}, enableRule(rule))
	assert.NilError(t, err)
	assert.Equal(t, len(violations), 1)

	suppressed := writeFile(t, t.TempDir(), "suppressed.graphql",
		declaration+"# eslint-disable-next-line @graphql-eslint/unique-directive-names-per-location\n"+
			"type Foo @foo @foo { a: String }\n")
	violations, err = Files([]string{suppressed}, enableRule(rule))
	assert.NilError(t, err)
	assert.Equal(t, len(violations), 0)
}

// TestWithoutSuppressedKeepsUnsuppressibleViolations confirms that, even
// under a directive covering every rule, withoutSuppressed keeps a Tier 1
// violation, an unclassified one, and one with no location, while still
// dropping a suppressible violation on the same line.
func TestWithoutSuppressedKeepsUnsuppressibleViolations(t *testing.T) {
	src := &ast.Source{Name: "s.graphql", Input: "# eslint-disable\ntype A { a: Int }\n"}
	at := func(rule string) Violation {
		return Violation{err: gqlerror.ErrorPosf(&ast.Position{Src: src, Line: 2, Column: 1}, "x"), Rule: rule}
	}
	noLocation := Violation{err: gqlerror.Errorf("no location"), Rule: config.RuleNamingConvention}

	got, err := withoutSuppressed([]*ast.Source{src}, []Violation{
		at(config.RuleKnownTypeNames), at(UnclassifiedRule), at(config.RuleNamingConvention), noLocation,
	})
	assert.NilError(t, err)

	rules := make([]string, 0, len(got))
	for _, v := range got {
		rules = append(rules, v.Rule)
	}
	assert.DeepEqual(t, rules, []string{config.RuleKnownTypeNames, UnclassifiedRule, config.RuleNamingConvention})
	assert.Equal(t, got[2].File(), "")
}

// TestFilesFromSourcesHonorsDirectives confirms suppression applies to
// sources supplied directly, as --diff mode supplies a merge-base commit's
// content, and stays within each source.
func TestFilesFromSourcesHonorsDirectives(t *testing.T) {
	sources := []*ast.Source{
		{Name: "a.graphql", Input: "enum A {\n\t# eslint-disable\n\tx\n}\n"},
		{Name: "b.graphql", Input: "enum B {\n\ty\n}\n"},
	}

	violations, err := FilesFromSources(sources, namingAndDescriptionCfg())
	assert.NilError(t, err)
	assert.Equal(t, len(violations), 2)
	for _, v := range violations {
		assert.Equal(t, v.File(), "b.graphql")
	}
}
