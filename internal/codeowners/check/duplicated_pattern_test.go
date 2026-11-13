// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Tests for the Duplicated Pattern check.
// Derived from github.com/mszostok/codeowners-validator (Apache License 2.0), with changes.

package check

import (
	"testing"

	"gotest.tools/v3/assert"
)

func TestDuplicatedPattern(t *testing.T) {
	tests := map[string]struct {
		codeownersInput string
		expectedIssues  []Issue
	}{
		"Should report info about duplicated entries in order of first appearance": {
			codeownersInput: `
					*       @global-owner1 @global-owner2

					/script @mszostok
					/build/logs/ @doctocat
					/build/logs/ @doctocat

					/script m.t@g.com
			`,
			expectedIssues: []Issue{
				{
					Severity: Error,
					Message: `Pattern "/script" is defined 2 times in lines:
            * 4: with owners: [@mszostok]
            * 8: with owners: [m.t@g.com]`,
				},
				{
					Severity: Error,
					Message: `Pattern "/build/logs/" is defined 2 times in lines:
            * 5: with owners: [@doctocat]
            * 6: with owners: [@doctocat]`,
				},
			},
		},
		"Should not report any issues with correct CODEOWNERS file": {
			codeownersInput: fixtureValidCodeowners,
		},
	}

	for tn, tc := range tests {
		t.Run(tn, func(t *testing.T) {
			out, err := NewDuplicatedPattern().Check(t.Context(), loadInput(t, tc.codeownersInput))
			assert.NilError(t, err)
			assert.DeepEqual(t, out.Issues, tc.expectedIssues)
		})
	}
}
