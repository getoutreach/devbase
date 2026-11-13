// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Tests for the Avoid Shadowing check.
// Derived from github.com/mszostok/codeowners-validator (Apache License 2.0), with changes.

package check

import (
	"testing"

	"gotest.tools/v3/assert"
)

func TestAvoidShadowing(t *testing.T) {
	tests := map[string]struct {
		codeownersInput string
		expectedIssues  []Issue
	}{
		"Should report info about shadowed entries": {
			codeownersInput: `
					/build/logs/ @doctocat
					/script      @mszostok

					# Shadows
					*            @s1
					/s*/         @s2
					/s*          @s3
					/b*          @s4
					/b*/logs     @s5

					# OK
					/b*/other    @o1
					/script/*	 @o2
			`,
			expectedIssues: []Issue{
				{
					Severity: Error,
					LineNo:   6,
					Message: `Pattern "*" shadows the following patterns:
            * 2: "/build/logs/"
            * 3: "/script"
Entries should go from least-specific to most-specific.`,
				},
				{
					Severity: Error,
					LineNo:   7,
					Message: `Pattern "/s*/" shadows the following patterns:
            * 3: "/script"
Entries should go from least-specific to most-specific.`,
				},
				{
					Severity: Error,
					LineNo:   8,
					Message: `Pattern "/s*" shadows the following patterns:
            * 3: "/script"
            * 7: "/s*/"
Entries should go from least-specific to most-specific.`,
				},
				{
					Severity: Error,
					LineNo:   9,
					Message: `Pattern "/b*" shadows the following patterns:
            * 2: "/build/logs/"
Entries should go from least-specific to most-specific.`,
				},
				{
					Severity: Error,
					LineNo:   10,
					Message: `Pattern "/b*/logs" shadows the following patterns:
            * 2: "/build/logs/"
Entries should go from least-specific to most-specific.`,
				},
			},
		},
		"Should not report any issues with correct CODEOWNERS file": {
			codeownersInput: fixtureValidCodeowners,
		},
	}

	for tn, tc := range tests {
		t.Run(tn, func(t *testing.T) {
			out, err := NewAvoidShadowing().Check(t.Context(), loadInput(t, tc.codeownersInput))
			assert.NilError(t, err)
			assert.DeepEqual(t, out.Issues, tc.expectedIssues)
		})
	}
}
