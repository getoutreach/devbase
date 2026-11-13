// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Tests for the Valid Syntax check.
// Derived from github.com/mszostok/codeowners-validator (Apache License 2.0), with changes.

package check

import (
	"testing"

	"github.com/getoutreach/devbase/v2/internal/codeowners"
	"gotest.tools/v3/assert"
)

func TestValidSyntaxChecker(t *testing.T) {
	tests := map[string]struct {
		codeowners string
		issue      *Issue
	}{
		"No owners": {
			codeowners: `*`,
		},
		"Bad username": {
			codeowners: `pkg/github.com/** @-`,
			issue: &Issue{
				Severity: Warning,
				LineNo:   1,
				Message:  "Owner '@-' does not look like a GitHub username or team name",
			},
		},
		"Bad org": {
			codeowners: `* @bad+org`,
			issue: &Issue{
				Severity: Warning,
				LineNo:   1,
				Message:  "Owner '@bad+org' does not look like a GitHub username or team name",
			},
		},
		"Bad team name on first place": {
			codeowners: `* @org/+not+a+good+name`,
			issue: &Issue{
				Severity: Warning,
				LineNo:   1,
				Message:  "Owner '@org/+not+a+good+name' does not look like a GitHub username or team name",
			},
		},
		"Bad team name on second place": {
			codeowners: `* @org/hakuna-matata @org/-a-team`,
			issue: &Issue{
				Severity: Warning,
				LineNo:   1,
				Message:  "Owner '@org/-a-team' does not look like a GitHub username or team name",
			},
		},
		"Doesn't look like username, team name, nor email": {
			codeowners: `* something_weird`,
			issue: &Issue{
				Severity: Error,
				LineNo:   1,
				Message:  "Owner 'something_weird' does not look like an email",
			},
		},
		"Comment in pattern line": {
			codeowners: `* @org/hakuna-matata # this is allowed`,
		},
	}
	for tn, tc := range tests {
		t.Run(tn, func(t *testing.T) {
			out, err := NewValidSyntax().Check(t.Context(), loadInput(t, tc.codeowners))
			assert.NilError(t, err)

			assertSingleIssue(t, tc.issue, out.Issues)
		})
	}
}

func TestValidSyntaxZeroValueEntry(t *testing.T) {
	in := Input{Entries: []codeowners.Entry{{}}}

	out, err := NewValidSyntax().Check(t.Context(), in)
	assert.NilError(t, err)

	assert.DeepEqual(t, out.Issues, []Issue{{Severity: Error, Message: "Missing pattern"}})
}
