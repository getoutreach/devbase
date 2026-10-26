// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Tests for "# devbase-lint-disable" style suppression comments.

package lint

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/getoutreach/devbase/v2/internal/graphql/config"
	"gotest.tools/v3/assert"
)

func invalidSuppression(keyword, problem string) string {
	return fmt.Sprintf("invalid-suppression Invalid suppression comment %s: %s", keyword, problem)
}

func unusedSuppression(keyword, rule string) string {
	return fmt.Sprintf("unused-suppression Unused suppression comment %s: rule %q suppressed no violation", keyword, rule)
}

// shadowedSuppression is unusedSuppression for a directive whose violations
// another directive already covers.
func shadowedSuppression(keyword, rule string) string {
	return unusedSuppression(keyword, rule) + " (another directive already covers it)"
}

func TestDevbaseLintSuppression(t *testing.T) {
	naming := func(v string) string { return fmt.Sprintf(namingFor, v) }
	desc := func(v string) string { return fmt.Sprintf(descFor, v) }

	const (
		next    = "devbase-lint-disable-next-line"
		both    = "naming-convention, require-description"
		noReas  = "has no reason (add \"-- reason\" or a \"# Why:\" comment directly above)"
		noRules = "names no rule"

		// A disable-line directive takes only an inline reason.
		noReasLine = "has no reason (add \"-- reason\")"
	)

	cases := []struct {
		name string
		sdl  string
		want []string
	}{
		{
			"next-line with an inline reason",
			`enum E {
				# devbase-lint-disable-next-line naming-convention, require-description -- Legacy value.
				first
				second
			}`,
			[]string{naming("second"), desc("second")},
		},
		{
			"next-line with a Why block above",
			`enum E {
				# Why: Legacy value,
				# kept for old clients.
				# devbase-lint-disable-next-line naming-convention, require-description
				first
			}`,
			nil,
		},
		{
			"comment lines before the Why line are ordinary comments",
			`enum E {
				# prose about first
				# Why: Legacy value.
				# devbase-lint-disable-next-line naming-convention, require-description
				first
			}`,
			nil,
		},
		{
			"a blank line ends the Why block, so the directive has no reason and suppresses nothing",
			`enum E {
				# Why: Legacy value.

				# devbase-lint-disable-next-line naming-convention, require-description
				first
			}`,
			[]string{invalidSuppression(next, noReas), naming("first"), desc("first")},
		},
		{
			"disable and enable bound a range",
			`enum E {
				# devbase-lint-disable naming-convention, require-description -- Generated section.
				a
				b
				# devbase-lint-enable naming-convention, require-description
				c
			}`,
			[]string{naming("c"), desc("c")},
		},
		{
			"enable with no rule re-enables every rule",
			`enum E {
				# devbase-lint-disable naming-convention, require-description -- Section.
				a
				# devbase-lint-enable
				b
			}`,
			[]string{naming("b"), desc("b")},
		},
		{
			"disable-line covers its own line",
			`enum E {
				a # devbase-lint-disable-line naming-convention, require-description -- Public name.
				b
			}`,
			[]string{naming("b"), desc("b")},
		},
		{
			"disable-line takes only an inline reason",
			`enum E {
				# Why: Public name.
				a # devbase-lint-disable-line naming-convention, require-description
			}`,
			[]string{invalidSuppression("devbase-lint-disable-line", noReasLine), naming("a"), desc("a")},
		},
		{
			"a directive without a reason fails closed",
			`enum E {
				# devbase-lint-disable-next-line ` + both + `
				first
			}`,
			[]string{invalidSuppression(next, noReas), naming("first"), desc("first")},
		},
		{
			"a directive that names no rule fails closed",
			`enum E {
				# devbase-lint-disable-next-line -- Because.
				first
			}`,
			[]string{invalidSuppression(next, noRules), naming("first"), desc("first")},
		},
		{
			"an unknown rule name fails closed",
			`enum E {
				# devbase-lint-disable-next-line nameing-convention -- Typo.
				first
			}`,
			[]string{invalidSuppression(next, `unknown rule "nameing-convention"`), naming("first"), desc("first")},
		},
		{
			"a Tier 1 rule cannot be named",
			`enum E {
				# devbase-lint-disable-next-line known-type-names -- Not allowed.
				first
			}`,
			[]string{invalidSuppression(next, `rule "known-type-names" cannot be suppressed`), naming("first"), desc("first")},
		},
		{
			"an unknown directive keyword is reported",
			`enum E {
				# devbase-lint-disabel-next-line ` + both + ` -- Typo.
				first
			}`,
			[]string{invalidSuppression("devbase-lint-disabel-next-line", "unknown directive"), naming("first"), desc("first")},
		},
		{
			"a directive that suppresses nothing for a rule is reported for that rule",
			`enum E {
				# devbase-lint-disable-next-line naming-convention, require-description -- Stale.
				FIRST
			}`,
			[]string{unusedSuppression(next, "naming-convention")},
		},
		{
			"a rule named twice is reported once",
			`enum E {
				# devbase-lint-disable-next-line naming-convention, naming-convention -- Stale.
				FIRST
			}`,
			[]string{desc("FIRST"), unusedSuppression(next, "naming-convention")},
		},
		{
			"a trailing disable takes only an inline reason",
			`enum E {
				# Why: Public name.
				a # devbase-lint-disable naming-convention, require-description
			}`,
			[]string{invalidSuppression("devbase-lint-disable", noReasLine), naming("a"), desc("a")},
		},
		{
			"a disable with no enable runs to the end of the file",
			`enum E {
				before
				# devbase-lint-disable naming-convention, require-description -- Rest of the file.
				a
				b
			}`,
			[]string{naming("before"), desc("before")},
		},
		{
			"a directive another one already covers is reported with that hint",
			`enum E {
				# devbase-lint-disable naming-convention -- Whole enum.
				a # devbase-lint-disable-line naming-convention, require-description -- Public.
			}`,
			[]string{shadowedSuppression("devbase-lint-disable", "naming-convention")},
		},
		{
			"a reason separator without whitespace is called out",
			`enum E {
				# devbase-lint-disable-next-line naming-convention-- Typo.
				first
			}`,
			[]string{
				invalidSuppression(next, `unknown rule "naming-convention-- Typo." (separate the reason with " -- "); `+noReas),
				naming("first"), desc("first"),
			},
		},
		{
			"a description below the directive is skipped",
			`enum E {
				# devbase-lint-disable-next-line naming-convention -- Legacy.
				"""
				A described value.
				"""
				legacy
			}`,
			nil,
		},
		{
			"a one-line description below the directive is skipped",
			`enum E {
				# devbase-lint-disable-next-line naming-convention -- Legacy.
				"A described value."
				legacy
			}`,
			nil,
		},
		{
			"a blank line before the description means the directive covers nothing",
			`enum E {
				# devbase-lint-disable-next-line naming-convention -- Legacy.

				"A described value."
				legacy
			}`,
			[]string{naming("legacy"), unusedSuppression(next, "naming-convention")},
		},
		{
			"an eslint-disable cannot silence the suppression diagnostics",
			`# eslint-disable
			enum E {
				# devbase-lint-disable-next-line ` + both + `
				first
			}`,
			[]string{invalidSuppression(next, noReas)},
		},
		{
			"a Tier 1 violation returns before any suppression diagnostics",
			`# devbase-lint-disable-next-line naming-convention -- Anything.
			type Foo { a: Missing }`,
			[]string{"known-type-names Undefined type Missing."},
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

// TestDevbaseLintDisableNextLineCoversDescriptionStyle confirms a
// disable-next-line above a description also covers the description's own
// line, where description-style reports, without covering a later one.
func TestDevbaseLintDisableNextLineCoversDescriptionStyle(t *testing.T) {
	cfg := enableRuleWithOptions(config.RuleDescriptionStyle, map[string]any{"style": "inline"})

	for name, sdl := range map[string]string{
		"a one-line block description": `# devbase-lint-disable-next-line description-style -- Legacy.
"""Legacy"""
type Foo { a: String }`,
		"a multi-line block description": `# devbase-lint-disable-next-line description-style -- Legacy.
"""
Legacy
"""
type Foo { a: String }`,
	} {
		t.Run(name, func(t *testing.T) {
			got := ruleMessages(t, sdl, cfg)
			assert.Equal(t, len(got), 0, "got %v", got)
		})
	}

	t.Run("a later description is not covered", func(t *testing.T) {
		got := ruleMessages(t, `# devbase-lint-disable-next-line description-style -- Legacy.
type Foo { a: String }

"""Other"""
type Bar { a: String }`, cfg)
		assert.Equal(t, len(got), 2, "got %v", got)
		assert.Assert(t, strings.HasPrefix(got[0], "description-style Unexpected block description"), "got %v", got)
		assert.Equal(t, got[1], unusedSuppression("devbase-lint-disable-next-line", "description-style"))
	})
}

// TestDevbaseLintSuppressionDiagnosticsCanBeTurnedOff confirms
// scripts/devbase.yaml can set either diagnostic to "off".
func TestDevbaseLintSuppressionDiagnosticsCanBeTurnedOff(t *testing.T) {
	cfg := namingAndDescriptionCfg()
	cfg.Rules[config.RuleInvalidSuppression] = config.Rule{Severity: config.SeverityOff}
	cfg.Rules[config.RuleUnusedSuppression] = config.Rule{Severity: config.SeverityOff}

	got := ruleMessages(t, `enum E {
		# devbase-lint-disable-next-line naming-convention, require-description
		first
		# devbase-lint-disable-next-line naming-convention, require-description -- Stale.
		SECOND_NAMED
	}`, cfg)

	// The directive without a reason suppresses nothing, so "first" keeps
	// both violations. The stale directive's unused report is silenced.
	want := []string{fmt.Sprintf(namingFor, "first"), fmt.Sprintf(descFor, "first")}
	slices.Sort(want)
	assert.DeepEqual(t, got, want)
}

// TestDevbaseLintSuppressionIsPerFile confirms a directive in one file does
// not suppress a violation in another file.
func TestDevbaseLintSuppressionIsPerFile(t *testing.T) {
	dir := t.TempDir()
	a := writeFile(t, dir, "a.graphql", "enum A {\n\t# devbase-lint-disable naming-convention, require-description -- Why.\n\tx\n}\n")
	b := writeFile(t, dir, "b.graphql", "enum B {\n\ty\n}\n")

	violations, err := Files([]string{a, b}, namingAndDescriptionCfg())
	assert.NilError(t, err)
	assert.Equal(t, len(violations), 2)
	for _, v := range violations {
		assert.Equal(t, v.File(), b)
	}
}

// TestDevbaseLintDirectiveIsNotAHashtagDescription confirms
// no-hashtag-description does not report a native directive, or the Why
// block above it, as a description.
func TestDevbaseLintDirectiveIsNotAHashtagDescription(t *testing.T) {
	cfg := &config.Lint{Rules: map[string]config.Rule{
		config.RuleNamingConvention:     {Severity: config.SeverityError, Options: map[string]any{"types": "PascalCase"}},
		config.RuleNoHashtagDescription: {Severity: config.SeverityError},
	}}

	got := ruleMessages(t, `# Why: Legacy name.
# devbase-lint-disable-next-line naming-convention
type bad_name { a: String }`, cfg)

	assert.Equal(t, len(got), 0, "got %v", got)
}

// TestDevbaseLintSuppressionLineEndings confirms a description below a
// directive is skipped whichever line ending the file uses, including the
// lone "\r" the lexer also treats as a line break, and with non-ASCII text
// before it.
func TestDevbaseLintSuppressionLineEndings(t *testing.T) {
	const sdl = "# café ☃\nenum E {\n\t# devbase-lint-disable-next-line naming-convention -- Legacy.\n" +
		"\t\"\"\"\n\tA described value.\n\t\"\"\"\n\tlegacy\n}\n"

	for name, newline := range map[string]string{"LF": "\n", "CRLF": "\r\n", "CR": "\r"} {
		t.Run(name, func(t *testing.T) {
			got := ruleMessages(t, strings.ReplaceAll(sdl, "\n", newline), namingAndDescriptionCfg())
			assert.Equal(t, len(got), 0, "got %v", got)
		})
	}
}
