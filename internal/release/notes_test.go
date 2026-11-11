// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Tests for the release notes renderer.

package release

import (
	"strings"
	"testing"
	"time"

	"gotest.tools/v3/assert"
)

// fakeHash returns a 40-character fake commit hash made of one digit.
func fakeHash(d string) string {
	return strings.Repeat(d, 40)
}

// TestRenderNotesMatchesConventionalCommits checks the output against
// what @semantic-release/release-notes-generator 14.1 printed for the
// same commits with conventional-changelog-conventionalcommits 9.3.
func TestRenderNotesMatchesConventionalCommits(t *testing.T) {
	commits := ParseCommits([]RawCommit{
		{Hash: fakeHash("1"), Message: "feat(api): add widget endpoint (#42)"},
		{Hash: fakeHash("2"), Message: "fix: handle nil pointer\n\nCloses #7"},
		{Hash: fakeHash("3"), Message: "feat!: drop v1 config"},
		{Hash: fakeHash("4"), Message: "chore(deps): bump things"},
		{Hash: fakeHash("5"), Message: "docs: explain"},
		{Hash: fakeHash("6"), Message: "perf(db): faster query\n\nBREAKING CHANGE: index removed"},
		{Hash: fakeHash("7"), Message: "Merge pull request #99 from org/branch"},
		{Hash: fakeHash("8"), Message: "revert: feat: something old"},
		{Hash: fakeHash("9"), Message: "feature: alt feature word"},
	})
	got := RenderNotes(&NotesInput{
		Version:     "1.3.0",
		PreviousTag: "v1.2.0",
		Date:        time.Date(2026, 11, 11, 23, 0, 0, 0, time.UTC),
		Commits:     commits,
		Links:       GitHubLinks{RepoURL: "https://github.com/acme/widgets"},
	})

	//nolint:lll // Why: the expected output has long lines.
	want := `## [1.3.0](https://github.com/acme/widgets/compare/v1.2.0...v1.3.0) (2026-11-11)

### ⚠ BREAKING CHANGES

* drop v1 config
* **db:** index removed

### Features

* alt feature word ([9999999](https://github.com/acme/widgets/commit/9999999999999999999999999999999999999999))
* **api:** add widget endpoint ([#42](https://github.com/acme/widgets/issues/42)) ([1111111](https://github.com/acme/widgets/commit/1111111111111111111111111111111111111111))
* drop v1 config ([3333333](https://github.com/acme/widgets/commit/3333333333333333333333333333333333333333))

### Bug Fixes

* handle nil pointer ([2222222](https://github.com/acme/widgets/commit/2222222222222222222222222222222222222222)), closes [#7](https://github.com/acme/widgets/issues/7)

### Performance Improvements

* **db:** faster query ([6666666](https://github.com/acme/widgets/commit/6666666666666666666666666666666666666666))

### Reverts

* feat: something old ([8888888](https://github.com/acme/widgets/commit/8888888888888888888888888888888888888888))
`
	assert.Equal(t, got, want)
}

func TestRenderNotesFirstRelease(t *testing.T) {
	got := RenderNotes(&NotesInput{
		Version: "1.0.0",
		Date:    time.Date(2026, 11, 11, 0, 0, 0, 0, time.UTC),
		Commits: ParseCommits([]RawCommit{{Hash: fakeHash("a"), Message: "feat: init"}}),
		Links:   GitHubLinks{RepoURL: "https://github.com/acme/widgets"},
	})
	assert.Equal(t, strings.SplitN(got, "\n", 2)[0], "## 1.0.0 (2026-11-11)")
}

func TestRepoURLFromRemote(t *testing.T) {
	for _, remote := range []string{
		"git@github.com:acme/widgets.git",
		"https://github.com/acme/widgets.git",
		"https://x-access-token:secret@github.com/acme/widgets",
		"ssh://git@github.com:22/acme/widgets.git",
	} {
		got, err := RepoURLFromRemote(remote)
		assert.NilError(t, err)
		assert.Equal(t, got, "https://github.com/acme/widgets", remote)
	}
}
