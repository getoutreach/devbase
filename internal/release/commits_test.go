// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Tests for commit parsing and analysis.

package release

import (
	"testing"

	"github.com/getoutreach/devbase/v2/internal/release/releaserc"
	"gotest.tools/v3/assert"
)

// customRules are the release rules that stencil-base renders when
// major versions are blocked.
func customRules() []releaserc.Rule {
	return []releaserc.Rule{
		{Breaking: true, Release: "minor"},
		{Type: "revert", Release: "patch"},
		{Type: "perf", Release: "patch"},
	}
}

func TestParseCommit(t *testing.T) {
	c := ParseCommit(RawCommit{Hash: "a1", Message: "feat(api)!: new api\n\nCloses #12"})
	assert.Equal(t, c.Type, "feat")
	assert.Equal(t, c.Scope, "api")
	assert.Equal(t, c.Subject, "new api")
	assert.DeepEqual(t, c.Notes, []string{"new api"})
	assert.DeepEqual(t, c.Closes, []string{"12"})

	c = ParseCommit(RawCommit{Hash: "a2", Message: "fix: x\n\nBREAKING-CHANGE: the api moved\nto v2\n\nRefs: y"})
	assert.DeepEqual(t, c.Notes, []string{"the api moved\nto v2"})

	c = ParseCommit(RawCommit{Hash: "a3", Message: "Revert \"feat: old\"\n\nThis reverts commit abc123."})
	assert.Equal(t, c.Type, "")
	assert.DeepEqual(t, c.Revert, &Revert{Header: "feat: old", Hash: "abc123"})
}

func TestParseCommitsDropsRevertedPairs(t *testing.T) {
	commits := ParseCommits([]RawCommit{
		{Hash: "b2", Message: "Revert \"feat: old\"\n\nThis reverts commit abc123."},
		{Hash: "abc123ffff", Message: "feat: old"},
		{Hash: "c3", Message: "fix: kept"},
		{Hash: "d4", Message: ""},
	})
	assert.Equal(t, len(commits), 1)
	assert.Equal(t, commits[0].Hash, "c3")
}

func TestAnalyze(t *testing.T) {
	tests := []struct {
		name    string
		message string
		rules   []releaserc.Rule
		want    Bump
	}{
		{"feat", "feat: x", nil, BumpMinor},
		{"fix", "fix: x", nil, BumpPatch},
		{"chore", "chore: x", nil, BumpNone},
		{"breaking default", "feat!: x", nil, BumpMajor},
		{"breaking footer default", "fix: x\n\nBREAKING CHANGE: y", nil, BumpMajor},
		{"breaking custom", "feat!: x", customRules(), BumpMinor},
		{"skip release", "feat: x [skip release]", nil, BumpNone},
		{"release false", "feat: x", []releaserc.Rule{{Type: "feat", Release: "false"}}, BumpNone},
		{"scope glob", "fix(core-db): x", []releaserc.Rule{{Scope: "core-*", Release: "minor"}}, BumpMinor},
		{"revert default", "Revert \"feat: x\"\n\nThis reverts commit abc.", nil, BumpPatch},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			commits := ParseCommits([]RawCommit{{Hash: "h", Message: tt.message}})
			assert.Equal(t, Analyze(commits, tt.rules), tt.want)
		})
	}

	// A custom rule match skips the default rules, so the breaking note
	// does not make this a major release.
	perf := ParseCommits([]RawCommit{{Hash: "h", Message: "perf: x\n\nBREAKING CHANGE: y"}})
	assert.Equal(t, Analyze(perf, []releaserc.Rule{{Type: "perf", Release: "patch"}}), BumpPatch)
}
