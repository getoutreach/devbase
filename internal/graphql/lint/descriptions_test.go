// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Shared test helper for the Tier 3 require-description
// and description-style rule tests, plus a test of the two running
// together.

package lint

import (
	"testing"

	"github.com/getoutreach/devbase/v2/internal/graphql/config"
	"gotest.tools/v3/assert"
)

// enableRuleWithOptions returns a Lint with rule set to
// SeverityError and opts as its Rule.Options, the minimum
// scripts/devbase.yaml entry needed to turn on a Tier 3 rule that
// takes options.
func enableRuleWithOptions(rule string, opts map[string]any) *config.Lint {
	return &config.Lint{Rules: map[string]config.Rule{rule: {Severity: config.SeverityError, Options: opts}}}
}

// TestDescriptionStyleAndRequireDescriptionBothRun confirms the two
// rules operate independently in the same Files call: a missing
// description trips require-description, and a present description in
// the wrong style trips description-style, in the same run.
func TestDescriptionStyleAndRequireDescriptionBothRun(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "schema.graphql", `
		"""
		Foo description
		"""
		type Foo {
			a: String
		}
	`)

	cfg := &config.Lint{Rules: map[string]config.Rule{
		config.RuleRequireDescription: {Severity: config.SeverityError, Options: map[string]any{"types": true, "FieldDefinition": true}},
		config.RuleDescriptionStyle:   {Severity: config.SeverityError, Options: map[string]any{"style": "inline"}},
	}}
	violations, err := Files([]string{path}, cfg)
	assert.NilError(t, err)
	assert.Equal(t, len(violations), 2)

	rules := make([]string, 0, len(violations))
	for _, v := range violations {
		rules = append(rules, v.Rule)
	}
	assert.Equal(t, len(rules), 2)
	assert.Equal(t, rules[0] != rules[1], true)
}
