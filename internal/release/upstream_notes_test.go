// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Release notes tests ported from the upstream tests of
// conventional-changelog-conventionalcommits 9.3.1,
// conventional-changelog-writer 8.4.0 and
// @semantic-release/release-notes-generator 14.1.1. The expected output of
// each case is what release-notes-generator gives in Node.js for the same
// commits, with the conventionalcommits preset and the same date.

package release

import (
	"cmp"
	"testing"
	"time"

	"github.com/getoutreach/devbase/v2/internal/release/forge/github"
	"gotest.tools/v3/assert"
)

// upstreamNotesCase is a release notes case ported from an upstream test.
// The tag of the release is "v" and the version.
type upstreamNotesCase struct {
	// name names the case in the subtest name.
	name string

	// repoURL is the repository URL, as RepoURLFromRemote returns it.
	// Empty means https://github.com/owner/repo.
	repoURL string

	// previousTag is the tag of the last release. Empty means v1.0.0.
	previousTag string

	// firstRelease means that there is no previous tag.
	firstRelease bool

	// version is the version of the release. Empty means 2.0.0.
	version string

	// commits are newest first, as git log gives them.
	commits []RawCommit

	// want is the release-notes-generator output in Node.js.
	want string
}

// upstreamNotesDate is the date that the Node.js run used.
var upstreamNotesDate = time.Date(2026, 11, 11, 12, 0, 0, 0, time.UTC)

// runUpstreamNotesCases renders the notes of each case and compares them
// with the Node.js output.
func runUpstreamNotesCases(t *testing.T, cases []upstreamNotesCase) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			previousTag := cmp.Or(tc.previousTag, "v1.0.0")
			if tc.firstRelease {
				previousTag = ""
			}
			version := cmp.Or(tc.version, "2.0.0")
			got := RenderNotes(&NotesInput{
				Version:     version,
				Tag:         "v" + version,
				PreviousTag: previousTag,
				Date:        upstreamNotesDate,
				Commits:     ParseCommits(tc.commits),
				Links:       github.NewLinks(cmp.Or(tc.repoURL, "https://github.com/owner/repo")),
			})
			assert.Equal(t, got, tc.want)
		})
	}
}

// notesPresetRepoURL is the repository of most preset cases.
const notesPresetRepoURL = "https://github.com/conventional-changelog/conventional-changelog"

// notesPresetSetups are the commits of the git setups in
// conventional-changelog-conventionalcommits test/index.spec.js, oldest
// first. A message with paragraphs comes from several "-m" options.
// Upstream tags v0.1.0 after setup 5 and v0.2.0 after setup 6.
//
//nolint:lll // Why: read-only test data with long commit messages.
var notesPresetSetups = [][]string{
	// Setup 1.
	{
		"build!: first build setup\n\nBREAKING CHANGE: New build system.",
		"ci(travis): add TravisCI pipeline\n\nBREAKING CHANGE: Continuously integrated.",
		"Feat: amazing new module\n\nBREAKING CHANGE: Not backward compatible.",
		"Fix(compile): avoid a bug\n\nBREAKING CHANGE: The Change is huge.",
		"perf(ngOptions): make it faster\n\n closes #1, #2",
		"fix(changelog): proper issue links\n\n see #1, conventional-changelog/standard-version#358",
		"revert(ngOptions): bad commit",
		"fix(*): oops",
		"fix(changelog): proper issue links\n\n see GH-1",
		"feat(awesome): adress EXAMPLE-1",
		"chore(deps): upgrade example from 1 to 2",
		"chore(release): release 0.0.0",
	},
	// Setup 2.
	{
		"feat(awesome): addresses the issue brought up in #133",
		"feat(awesome): addresses the issue brought up in #1a2b",
	},
	// Setup 3.
	{
		"feat(awesome): fix #88",
	},
	// Setup 4.
	{
		"feat(awesome): issue brought up by @bcoe! on Friday",
	},
	// Setup 5.
	{
		"build(npm): edit build script\n\nBREAKING CHANGE: The Change is huge.",
		"ci(travis): setup travis\n\nBREAKING CHANGE: The Change is huge.",
		"docs(readme): make it clear\n\nBREAKING CHANGE: The Change is huge.",
		"style(whitespace): make it easier to read\n\nBREAKING CHANGE: The Change is huge.",
		"refactor(code): change a lot of code\n\nBREAKING CHANGE: The Change is huge.",
		"test(*)!: more tests\n\nBREAKING CHANGE: The Change is huge.",
	},
	// Setup 6.
	{
		"feat: some more feats",
	},
	// Setup 7.
	{
		"feature: some more features",
	},
	// Setup 8.
	{
		"feat(*): implementing #5 by @dlmr\n\n closes #10",
	},
	// Setup 9.
	{
		"fix: use npm@5 (@username)",
		"build(deps): bump @dummy/package from 7.1.2 to 8.0.0\n\nBREAKING CHANGE: The Change is huge.",
		"feat: complex new feature\n\nthis is a complex new feature with many reviewers\n\nReviewer: @hutson\n\nFixes: #99\n\nRefs: #100\n\nBREAKING CHANGE: this completely changes the API",
		"FEAT(foo)!: incredible new flag FIXES: #33",
	},
	// Setup 10.
	{
		"Revert \"feat: default revert format\"\n\nThis reverts commit 1234.",
		"revert: feat: custom revert format\n\nThis reverts commit 5678.",
	},
	// Setup 11.
	{
		"chore: release at different version\n\nRelease-As: v3.0.2",
	},
	// Setup 12.
	{
		"fix: replace `@nano_kit/router` with `@nano_kit/router2`",
	},
}

// notesPresetLog returns the commits of setups from to to (1-based), newest
// first, as git log gives them. Each commit has its own fake hash.
func notesPresetLog(from, to int) []RawCommit {
	const hashChars = "123456789abcdefghijklmnopqrstuvwxyz"
	var commits []RawCommit
	k := 0
	for i, setup := range notesPresetSetups {
		for _, msg := range setup {
			if i+1 >= from && i+1 <= to {
				commits = append([]RawCommit{{Hash: fakeHash(hashChars[k : k+1]), Message: msg}}, commits...)
			}
			k++
		}
	}
	return commits
}

// TestUpstreamConventionalCommitsPreset ports the applicable tests of
// conventional-changelog-conventionalcommits test/index.spec.js (9.3.1). Each case feeds the
// commits of the upstream git setups, newest first, from the last tag.
// When upstream checks only the commits of the last setup, the case
// feeds only that setup.
func TestUpstreamConventionalCommitsPreset(t *testing.T) {
	//nolint:lll // Why: the expected outputs and upstream titles have long lines.
	cases := []upstreamNotesCase{
		// Upstream:
		//   conventional-changelog-conventionalcommits test/index.spec.js: "should work if there is no semver tag"
		//   conventional-changelog-conventionalcommits test/index.spec.js: "should not list breaking change twice if ! is used"
		//   conventional-changelog-conventionalcommits test/index.spec.js: "should properly format external repository issues"
		// Upstream checks that the sections are present and in the order
		// BREAKING CHANGE, Features, Bug Fixes, Performance Improvements,
		// Reverts; that "* first build setup" is not listed twice; and that
		// #1 and conventional-changelog/standard-version#358 link to their
		// repositories.
		{
			name:         "no semver tag",
			repoURL:      notesPresetRepoURL,
			firstRelease: true,
			version:      "1.0.0",
			commits:      notesPresetLog(1, 1),
			want: `## 1.0.0 (2026-11-11)

### ⚠ BREAKING CHANGES

* **compile:** The Change is huge.
* Not backward compatible.
* **travis:** Continuously integrated.
* New build system.

### Features

* amazing new module ([3333333](https://github.com/conventional-changelog/conventional-changelog/commit/3333333333333333333333333333333333333333))
* **awesome:** adress EXAMPLE-1 ([aaaaaaa](https://github.com/conventional-changelog/conventional-changelog/commit/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa))

### Bug Fixes

* **changelog:** proper issue links ([9999999](https://github.com/conventional-changelog/conventional-changelog/commit/9999999999999999999999999999999999999999))
* **changelog:** proper issue links ([6666666](https://github.com/conventional-changelog/conventional-changelog/commit/6666666666666666666666666666666666666666)), closes [#1](https://github.com/conventional-changelog/conventional-changelog/issues/1) [conventional-changelog/standard-version#358](https://github.com/conventional-changelog/standard-version/issues/358)
* **compile:** avoid a bug ([4444444](https://github.com/conventional-changelog/conventional-changelog/commit/4444444444444444444444444444444444444444))
* oops ([8888888](https://github.com/conventional-changelog/conventional-changelog/commit/8888888888888888888888888888888888888888))

### Performance Improvements

* **ngOptions:** make it faster ([5555555](https://github.com/conventional-changelog/conventional-changelog/commit/5555555555555555555555555555555555555555)), closes [#1](https://github.com/conventional-changelog/conventional-changelog/issues/1) [#2](https://github.com/conventional-changelog/conventional-changelog/issues/2)

### Reverts

* **ngOptions:** bad commit ([7777777](https://github.com/conventional-changelog/conventional-changelog/commit/7777777777777777777777777777777777777777))

### Build System

* first build setup ([1111111](https://github.com/conventional-changelog/conventional-changelog/commit/1111111111111111111111111111111111111111))

### Continuous Integration

* **travis:** add TravisCI pipeline ([2222222](https://github.com/conventional-changelog/conventional-changelog/commit/2222222222222222222222222222222222222222))
`,
		},
		// Upstream:
		//   conventional-changelog-conventionalcommits test/index.spec.js: "should replace #[a-z0-9]+ with issue URL by default"
		// Upstream checks that #133 and #1a2b in subjects become issue links.
		{
			name:         "issue link in subject",
			repoURL:      notesPresetRepoURL,
			firstRelease: true,
			version:      "1.0.0",
			commits:      notesPresetLog(2, 2),
			want: `## 1.0.0 (2026-11-11)

### Features

* **awesome:** addresses the issue brought up in [#133](https://github.com/conventional-changelog/conventional-changelog/issues/133) ([ddddddd](https://github.com/conventional-changelog/conventional-changelog/commit/dddddddddddddddddddddddddddddddddddddddd))
* **awesome:** addresses the issue brought up in [#1a2b](https://github.com/conventional-changelog/conventional-changelog/issues/1a2b) ([eeeeeee](https://github.com/conventional-changelog/conventional-changelog/commit/eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee))
`,
		},
		// Upstream:
		//   conventional-changelog-conventionalcommits test/index.spec.js: "should remove the issues that already appear in the subject"
		// Upstream checks that #88 is linked in the subject and not repeated after "closes".
		{
			name:         "subject issue not repeated",
			repoURL:      notesPresetRepoURL,
			firstRelease: true,
			version:      "1.0.0",
			commits:      notesPresetLog(3, 3),
			want: `## 1.0.0 (2026-11-11)

### Features

* **awesome:** fix [#88](https://github.com/conventional-changelog/conventional-changelog/issues/88) ([fffffff](https://github.com/conventional-changelog/conventional-changelog/commit/ffffffffffffffffffffffffffffffffffffffff))
`,
		},
		// Upstream:
		//   conventional-changelog-conventionalcommits test/index.spec.js: "should replace @user with configured userUrlFormat"
		// Upstream sets a custom userUrlFormat, which the port does not support.
		// This case keeps the input and checks the default user URL for @bcoe.
		{
			name:         "user link",
			repoURL:      notesPresetRepoURL,
			firstRelease: true,
			version:      "1.0.0",
			commits:      notesPresetLog(4, 4),
			want: `## 1.0.0 (2026-11-11)

### Features

* **awesome:** issue brought up by [@bcoe](https://github.com/bcoe)! on Friday ([ggggggg](https://github.com/conventional-changelog/conventional-changelog/commit/gggggggggggggggggggggggggggggggggggggggg))
`,
		},
		// Upstream:
		//   conventional-changelog-conventionalcommits test/index.spec.js: "should not discard commit if there is BREAKING CHANGE"
		//   conventional-changelog-conventionalcommits test/index.spec.js: "should omit optional ! in breaking commit"
		// Upstream checks that hidden types with a breaking change get their
		// sections, and that "test(*)!" gives "### Tests" and "* more tests".
		{
			name:         "hidden types with breaking change",
			repoURL:      notesPresetRepoURL,
			firstRelease: true,
			version:      "1.0.0",
			commits:      notesPresetLog(5, 5),
			want: `## 1.0.0 (2026-11-11)

### ⚠ BREAKING CHANGES

* The Change is huge.
* **code:** The Change is huge.
* **whitespace:** The Change is huge.
* **readme:** The Change is huge.
* **travis:** The Change is huge.
* **npm:** The Change is huge.

### Documentation

* **readme:** make it clear ([jjjjjjj](https://github.com/conventional-changelog/conventional-changelog/commit/jjjjjjjjjjjjjjjjjjjjjjjjjjjjjjjjjjjjjjjj))

### Styles

* **whitespace:** make it easier to read ([kkkkkkk](https://github.com/conventional-changelog/conventional-changelog/commit/kkkkkkkkkkkkkkkkkkkkkkkkkkkkkkkkkkkkkkkk))

### Code Refactoring

* **code:** change a lot of code ([lllllll](https://github.com/conventional-changelog/conventional-changelog/commit/llllllllllllllllllllllllllllllllllllllll))

### Tests

* more tests ([mmmmmmm](https://github.com/conventional-changelog/conventional-changelog/commit/mmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmm))

### Build System

* **npm:** edit build script ([hhhhhhh](https://github.com/conventional-changelog/conventional-changelog/commit/hhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhh))

### Continuous Integration

* **travis:** setup travis ([iiiiiii](https://github.com/conventional-changelog/conventional-changelog/commit/iiiiiiiiiiiiiiiiiiiiiiiiiiiiiiiiiiiiiiii))
`,
		},
		// Upstream:
		//   conventional-changelog-conventionalcommits test/index.spec.js: "should work if there is a semver tag"
		// Upstream checks that only "some more feats" is listed, with no BREAKING section.
		{
			name:        "semver tag",
			repoURL:     notesPresetRepoURL,
			previousTag: "v0.1.0",
			version:     "0.2.0",
			commits:     notesPresetLog(6, 6),
			want: `## [0.2.0](https://github.com/conventional-changelog/conventional-changelog/compare/v0.1.0...v0.2.0) (2026-11-11)

### Features

* some more feats ([nnnnnnn](https://github.com/conventional-changelog/conventional-changelog/commit/nnnnnnnnnnnnnnnnnnnnnnnnnnnnnnnnnnnnnnnn))
`,
		},
		// Upstream:
		//   conventional-changelog-conventionalcommits test/index.spec.js: "should support \"feature\" as alias for \"feat\""
		// Upstream checks that "feature:" is listed, with no BREAKING section.
		{
			name:        "feature alias",
			repoURL:     notesPresetRepoURL,
			previousTag: "v0.2.0",
			version:     "0.3.0",
			commits:     notesPresetLog(7, 7),
			want: `## [0.3.0](https://github.com/conventional-changelog/conventional-changelog/compare/v0.2.0...v0.3.0) (2026-11-11)

### Features

* some more features ([ooooooo](https://github.com/conventional-changelog/conventional-changelog/commit/oooooooooooooooooooooooooooooooooooooooo))
`,
		},
		// Upstream:
		//   conventional-changelog-conventionalcommits test/index.spec.js: "should fallback to the closest package.json when not providing a location for a package.json"
		// Upstream reads the repository from package.json, which the port does
		// not do. This case keeps the link checks: compare, commit and issue
		// URLs of conventional-changelog/conventional-changelog.
		{
			name:        "links from repository URL",
			repoURL:     notesPresetRepoURL,
			previousTag: "v0.2.0",
			version:     "0.3.0",
			commits:     notesPresetLog(7, 8),
			want: `## [0.3.0](https://github.com/conventional-changelog/conventional-changelog/compare/v0.2.0...v0.3.0) (2026-11-11)

### Features

* implementing [#5](https://github.com/conventional-changelog/conventional-changelog/issues/5) by [@dlmr](https://github.com/dlmr) ([ppppppp](https://github.com/conventional-changelog/conventional-changelog/commit/pppppppppppppppppppppppppppppppppppppppp)), closes [#10](https://github.com/conventional-changelog/conventional-changelog/issues/10)
* some more features ([ooooooo](https://github.com/conventional-changelog/conventional-changelog/commit/oooooooooooooooooooooooooooooooooooooooo))
`,
		},
		// Upstream:
		//   conventional-changelog-conventionalcommits test/index.spec.js: "should work specifying where to find a package.json using conventional-changelog"
		// Upstream reads the repository from a given package.json, which the
		// port does not do. This case keeps the link checks: compare, commit and
		// issue URLs of conventional-changelog/example.
		{
			name:        "links from other repository URL",
			repoURL:     "https://github.com/conventional-changelog/example",
			previousTag: "v0.2.0",
			version:     "0.3.0",
			commits:     notesPresetLog(7, 8),
			want: `## [0.3.0](https://github.com/conventional-changelog/example/compare/v0.2.0...v0.3.0) (2026-11-11)

### Features

* implementing [#5](https://github.com/conventional-changelog/example/issues/5) by [@dlmr](https://github.com/dlmr) ([ppppppp](https://github.com/conventional-changelog/example/commit/pppppppppppppppppppppppppppppppppppppppp)), closes [#10](https://github.com/conventional-changelog/example/issues/10)
* some more features ([ooooooo](https://github.com/conventional-changelog/example/commit/oooooooooooooooooooooooooooooooooooooooo))
`,
		},
		// Upstream:
		//   conventional-changelog-conventionalcommits test/index.spec.js: "should only replace with link to user if it is an username"
		//   conventional-changelog-conventionalcommits test/index.spec.js: "supports multiple lines of footer information"
		//   conventional-changelog-conventionalcommits test/index.spec.js: "does not require that types are case sensitive"
		//   conventional-changelog-conventionalcommits test/index.spec.js: "populates breaking change if ! is present"
		// Upstream checks that @username is linked but npm@5 and
		// @dummy/package are not; that #99 and #100 are references and the
		// breaking note is listed; that "FEAT(foo)!" is listed; and that the
		// note "incredible new flag FIXES: #33" ends its line.
		{
			name:        "users, footers, type case and bang",
			repoURL:     notesPresetRepoURL,
			previousTag: "v0.2.0",
			version:     "1.0.0",
			commits:     notesPresetLog(7, 9),
			want: `## [1.0.0](https://github.com/conventional-changelog/conventional-changelog/compare/v0.2.0...v1.0.0) (2026-11-11)

### ⚠ BREAKING CHANGES

* **foo:** incredible new flag FIXES: #33
* this completely changes the API
* **deps:** The Change is huge.

### Features

* complex new feature ([sssssss](https://github.com/conventional-changelog/conventional-changelog/commit/ssssssssssssssssssssssssssssssssssssssss)), closes [#99](https://github.com/conventional-changelog/conventional-changelog/issues/99) [#100](https://github.com/conventional-changelog/conventional-changelog/issues/100)
* **foo:** incredible new flag FIXES: [#33](https://github.com/conventional-changelog/conventional-changelog/issues/33) ([ttttttt](https://github.com/conventional-changelog/conventional-changelog/commit/tttttttttttttttttttttttttttttttttttttttt))
* implementing [#5](https://github.com/conventional-changelog/conventional-changelog/issues/5) by [@dlmr](https://github.com/dlmr) ([ppppppp](https://github.com/conventional-changelog/conventional-changelog/commit/pppppppppppppppppppppppppppppppppppppppp)), closes [#10](https://github.com/conventional-changelog/conventional-changelog/issues/10)
* some more features ([ooooooo](https://github.com/conventional-changelog/conventional-changelog/commit/oooooooooooooooooooooooooooooooooooooooo))

### Bug Fixes

* use npm@5 ([@username](https://github.com/username)) ([qqqqqqq](https://github.com/conventional-changelog/conventional-changelog/commit/qqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqq))

### Build System

* **deps:** bump @dummy/package from 7.1.2 to 8.0.0 ([rrrrrrr](https://github.com/conventional-changelog/conventional-changelog/commit/rrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrr))
`,
		},
		// Upstream:
		//   conventional-changelog-conventionalcommits test/index.spec.js: "parses both default (Revert \"<subject>\") and custom (revert: <subject>) revert commits"
		// Upstream checks that both revert commits are listed.
		{
			name:        "revert formats",
			repoURL:     notesPresetRepoURL,
			previousTag: "v0.2.0",
			version:     "1.0.0",
			commits:     notesPresetLog(7, 10),
			want: `## [1.0.0](https://github.com/conventional-changelog/conventional-changelog/compare/v0.2.0...v1.0.0) (2026-11-11)

### ⚠ BREAKING CHANGES

* **foo:** incredible new flag FIXES: #33
* this completely changes the API
* **deps:** The Change is huge.

### Features

* complex new feature ([sssssss](https://github.com/conventional-changelog/conventional-changelog/commit/ssssssssssssssssssssssssssssssssssssssss)), closes [#99](https://github.com/conventional-changelog/conventional-changelog/issues/99) [#100](https://github.com/conventional-changelog/conventional-changelog/issues/100)
* **foo:** incredible new flag FIXES: [#33](https://github.com/conventional-changelog/conventional-changelog/issues/33) ([ttttttt](https://github.com/conventional-changelog/conventional-changelog/commit/tttttttttttttttttttttttttttttttttttttttt))
* implementing [#5](https://github.com/conventional-changelog/conventional-changelog/issues/5) by [@dlmr](https://github.com/dlmr) ([ppppppp](https://github.com/conventional-changelog/conventional-changelog/commit/pppppppppppppppppppppppppppppppppppppppp)), closes [#10](https://github.com/conventional-changelog/conventional-changelog/issues/10)
* some more features ([ooooooo](https://github.com/conventional-changelog/conventional-changelog/commit/oooooooooooooooooooooooooooooooooooooooo))

### Bug Fixes

* use npm@5 ([@username](https://github.com/username)) ([qqqqqqq](https://github.com/conventional-changelog/conventional-changelog/commit/qqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqq))

### Reverts

* Revert "feat: default revert format" ([uuuuuuu](https://github.com/conventional-changelog/conventional-changelog/commit/uuuuuuuuuuuuuuuuuuuuuuuuuuuuuuuuuuuuuuuu))
* feat: custom revert format ([vvvvvvv](https://github.com/conventional-changelog/conventional-changelog/commit/vvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvv))

### Build System

* **deps:** bump @dummy/package from 7.1.2 to 8.0.0 ([rrrrrrr](https://github.com/conventional-changelog/conventional-changelog/commit/rrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrr))
`,
		},
		// Upstream:
		//   conventional-changelog-conventionalcommits test/index.spec.js: "should include commits with \"Release-As:\" footer in CHANGELOG"
		// Upstream checks that the hidden chore with a Release-As footer is listed.
		{
			name:        "Release-As footer",
			repoURL:     notesPresetRepoURL,
			previousTag: "v0.2.0",
			version:     "1.0.0",
			commits:     notesPresetLog(7, 11),
			want: `## [1.0.0](https://github.com/conventional-changelog/conventional-changelog/compare/v0.2.0...v1.0.0) (2026-11-11)

### ⚠ BREAKING CHANGES

* **foo:** incredible new flag FIXES: #33
* this completely changes the API
* **deps:** The Change is huge.

### Features

* complex new feature ([sssssss](https://github.com/conventional-changelog/conventional-changelog/commit/ssssssssssssssssssssssssssssssssssssssss)), closes [#99](https://github.com/conventional-changelog/conventional-changelog/issues/99) [#100](https://github.com/conventional-changelog/conventional-changelog/issues/100)
* **foo:** incredible new flag FIXES: [#33](https://github.com/conventional-changelog/conventional-changelog/issues/33) ([ttttttt](https://github.com/conventional-changelog/conventional-changelog/commit/tttttttttttttttttttttttttttttttttttttttt))
* implementing [#5](https://github.com/conventional-changelog/conventional-changelog/issues/5) by [@dlmr](https://github.com/dlmr) ([ppppppp](https://github.com/conventional-changelog/conventional-changelog/commit/pppppppppppppppppppppppppppppppppppppppp)), closes [#10](https://github.com/conventional-changelog/conventional-changelog/issues/10)
* some more features ([ooooooo](https://github.com/conventional-changelog/conventional-changelog/commit/oooooooooooooooooooooooooooooooooooooooo))

### Bug Fixes

* use npm@5 ([@username](https://github.com/username)) ([qqqqqqq](https://github.com/conventional-changelog/conventional-changelog/commit/qqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqq))

### Reverts

* Revert "feat: default revert format" ([uuuuuuu](https://github.com/conventional-changelog/conventional-changelog/commit/uuuuuuuuuuuuuuuuuuuuuuuuuuuuuuuuuuuuuuuu))
* feat: custom revert format ([vvvvvvv](https://github.com/conventional-changelog/conventional-changelog/commit/vvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvv))

### Miscellaneous Chores

* release at different version ([wwwwwww](https://github.com/conventional-changelog/conventional-changelog/commit/wwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwww))

### Build System

* **deps:** bump @dummy/package from 7.1.2 to 8.0.0 ([rrrrrrr](https://github.com/conventional-changelog/conventional-changelog/commit/rrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrr))
`,
		},
		// Upstream:
		//   conventional-changelog-conventionalcommits test/index.spec.js: "should not replace @mention inside backtick-wrapped code"
		// Upstream checks that `@nano_kit/router` stays as is and is not linked.
		{
			name:        "mention in code span",
			repoURL:     notesPresetRepoURL,
			previousTag: "v0.2.0",
			version:     "1.0.0",
			commits:     notesPresetLog(7, 12),
			want: `## [1.0.0](https://github.com/conventional-changelog/conventional-changelog/compare/v0.2.0...v1.0.0) (2026-11-11)

### ⚠ BREAKING CHANGES

* **foo:** incredible new flag FIXES: #33
* this completely changes the API
* **deps:** The Change is huge.

### Features

* complex new feature ([sssssss](https://github.com/conventional-changelog/conventional-changelog/commit/ssssssssssssssssssssssssssssssssssssssss)), closes [#99](https://github.com/conventional-changelog/conventional-changelog/issues/99) [#100](https://github.com/conventional-changelog/conventional-changelog/issues/100)
* **foo:** incredible new flag FIXES: [#33](https://github.com/conventional-changelog/conventional-changelog/issues/33) ([ttttttt](https://github.com/conventional-changelog/conventional-changelog/commit/tttttttttttttttttttttttttttttttttttttttt))
* implementing [#5](https://github.com/conventional-changelog/conventional-changelog/issues/5) by [@dlmr](https://github.com/dlmr) ([ppppppp](https://github.com/conventional-changelog/conventional-changelog/commit/pppppppppppppppppppppppppppppppppppppppp)), closes [#10](https://github.com/conventional-changelog/conventional-changelog/issues/10)
* some more features ([ooooooo](https://github.com/conventional-changelog/conventional-changelog/commit/oooooooooooooooooooooooooooooooooooooooo))

### Bug Fixes

* replace ` + "`" + `@nano_kit/router` + "`" + ` with ` + "`" + `@nano_kit/router2` + "`" + ` ([xxxxxxx](https://github.com/conventional-changelog/conventional-changelog/commit/xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx))
* use npm@5 ([@username](https://github.com/username)) ([qqqqqqq](https://github.com/conventional-changelog/conventional-changelog/commit/qqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqq))

### Reverts

* Revert "feat: default revert format" ([uuuuuuu](https://github.com/conventional-changelog/conventional-changelog/commit/uuuuuuuuuuuuuuuuuuuuuuuuuuuuuuuuuuuuuuuu))
* feat: custom revert format ([vvvvvvv](https://github.com/conventional-changelog/conventional-changelog/commit/vvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvv))

### Miscellaneous Chores

* release at different version ([wwwwwww](https://github.com/conventional-changelog/conventional-changelog/commit/wwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwww))

### Build System

* **deps:** bump @dummy/package from 7.1.2 to 8.0.0 ([rrrrrrr](https://github.com/conventional-changelog/conventional-changelog/commit/rrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrr))
`,
		},
	}
	runUpstreamNotesCases(t, cases)
}

// TestUpstreamReleaseNotesGenerator ports the applicable tests of
// release-notes-generator test/integration.test.js (14.1.1). Upstream uses
// the angular preset by default; the port supports only the
// conventionalcommits preset, so the expected output is the one for
// that preset.
func TestUpstreamReleaseNotesGenerator(t *testing.T) {
	cases := []upstreamNotesCase{
		// Upstream:
		//   release-notes-generator test/integration.test.js: "Use \"conventional-changelog-angular\" by default"
		// Upstream checks the compare link, the Bug Fixes and Features
		// sections, and the scoped commit lines with commit links.
		{
			name: "default preset",
			commits: []RawCommit{
				{Hash: "111", Message: "fix(scope1): First fix"},
				{Hash: "222", Message: "feat(scope2): Second feature"},
			},
			want: `## [2.0.0](https://github.com/owner/repo/compare/v1.0.0...v2.0.0) (2026-11-11)

### Features

* **scope2:** Second feature ([222](https://github.com/owner/repo/commit/222))

### Bug Fixes

* **scope1:** First fix ([111](https://github.com/owner/repo/commit/111))
`,
		},
		// Upstream:
		//   release-notes-generator test/integration.test.js: "Accept a custom repository URL with git+https format"
		// Upstream uses a domain.com URL, which is not a github.com host. This
		// case keeps the input on github.com and checks "closes #10" from
		// "resolve #10".
		{
			name: "resolve keyword",
			commits: []RawCommit{
				{Hash: "111", Message: "fix(scope1): First fix\n\nresolve #10"},
				{Hash: "222", Message: "feat(scope2): Second feature"},
			},
			want: `## [2.0.0](https://github.com/owner/repo/compare/v1.0.0...v2.0.0) (2026-11-11)

### Features

* **scope2:** Second feature ([222](https://github.com/owner/repo/commit/222))

### Bug Fixes

* **scope1:** First fix ([111](https://github.com/owner/repo/commit/111)), closes [#10](https://github.com/owner/repo/issues/10)
`,
		},
		// Upstream:
		//   release-notes-generator test/integration.test.js: "Accept a Bitbucket repository URL"
		// Upstream uses a bitbucket.org URL, which is not a github.com host.
		// This case keeps the input on github.com and checks "closes #10"
		// from "Resolves #10".
		{
			name: "Resolves keyword",
			commits: []RawCommit{
				{Hash: "111", Message: "fix(scope1): First fix\n\nResolves #10"},
				{Hash: "222", Message: "feat(scope2): Second feature"},
			},
			want: `## [2.0.0](https://github.com/owner/repo/compare/v1.0.0...v2.0.0) (2026-11-11)

### Features

* **scope2:** Second feature ([222](https://github.com/owner/repo/commit/222))

### Bug Fixes

* **scope1:** First fix ([111](https://github.com/owner/repo/commit/111)), closes [#10](https://github.com/owner/repo/issues/10)
`,
		},
		// Upstream:
		//   release-notes-generator test/integration.test.js: "Accept a Gitlab repository URL"
		// Upstream uses a gitlab.com URL, which is not a github.com host. This
		// case keeps the input on github.com and checks "closes #10" from
		// "closed #10".
		{
			name: "closed keyword",
			commits: []RawCommit{
				{Hash: "111", Message: "fix(scope1): First fix\n\nclosed #10"},
				{Hash: "222", Message: "feat(scope2): Second feature"},
			},
			want: `## [2.0.0](https://github.com/owner/repo/compare/v1.0.0...v2.0.0) (2026-11-11)

### Features

* **scope2:** Second feature ([222](https://github.com/owner/repo/commit/222))

### Bug Fixes

* **scope1:** First fix ([111](https://github.com/owner/repo/commit/111)), closes [#10](https://github.com/owner/repo/issues/10)
`,
		},
		// Upstream:
		//   release-notes-generator test/integration.test.js: "Ignore malformatted commits and include valid ones"
		// Upstream checks that the fix is listed and "Feature => Invalid message" is not.
		{
			name: "malformed commit",
			commits: []RawCommit{
				{Hash: "111", Message: "fix(scope1): First fix"},
				{Hash: "222", Message: "Feature => Invalid message"},
			},
			want: `## [2.0.0](https://github.com/owner/repo/compare/v1.0.0...v2.0.0) (2026-11-11)

### Bug Fixes

* **scope1:** First fix ([111](https://github.com/owner/repo/commit/111))
`,
		},
		// Upstream:
		//   release-notes-generator test/integration.test.js: "Exclude commits if they have a matching revert commits"
		// Upstream checks that the reverted feature and its revert are left out.
		{
			name: "reverted commit",
			commits: []RawCommit{
				{Hash: "df012f1", Message: "revert: feat(scope2): First feature\n\nThis reverts commit df012f2.\n"},
				{Hash: "df012f2", Message: "feat(scope2): First feature"},
				{Hash: "df012f3", Message: "fix(scope1): First fix"},
			},
			want: `## [2.0.0](https://github.com/owner/repo/compare/v1.0.0...v2.0.0) (2026-11-11)

### Bug Fixes

* **scope1:** First fix ([df012f3](https://github.com/owner/repo/commit/df012f3))
`,
		},
		// Upstream:
		//   release-notes-generator test/integration.test.js: "Exclude commits with empty message"
		// Upstream checks that commits 222 and 333 are left out.
		{
			name: "empty messages",
			commits: []RawCommit{
				{Hash: "111", Message: "fix(scope1): First fix"},
				{Hash: "222", Message: ""},
				{Hash: "333", Message: "  "},
			},
			want: `## [2.0.0](https://github.com/owner/repo/compare/v1.0.0...v2.0.0) (2026-11-11)

### Bug Fixes

* **scope1:** First fix ([111](https://github.com/owner/repo/commit/111))
`,
		},
	}
	runUpstreamNotesCases(t, cases)
}

// TestUpstreamChangelogWriter ports the conventional-changelog-writer
// (8.4.0) tests whose behavior the conventionalcommits template and
// transform reach. Upstream gives the writer parsed commits and generic
// options; this port gives raw commits and the preset's options.
func TestUpstreamChangelogWriter(t *testing.T) {
	//nolint:lll // Why: the expected outputs and upstream titles have long lines.
	cases := []upstreamNotesCase{
		// Upstream:
		//   conventional-changelog-writer src/writers.spec.ts: "should still work if there is no commits"
		// Upstream checks that only the header is written.
		{
			name: "no commits",
			want: `## [2.0.0](https://github.com/owner/repo/compare/v1.0.0...v2.0.0) (2026-11-11)
`,
		},
		// Upstream:
		//   conventional-changelog-writer src/writers.spec.ts: "should ignore the field if it doesn't exist"
		// Upstream checks that a commit with only a header is written as
		// "* bla". The preset transform drops a commit with no type.
		{
			name: "commit without type",
			commits: []RawCommit{
				{Hash: fakeHash("1"), Message: "bla"},
			},
			want: `## [2.0.0](https://github.com/owner/repo/compare/v1.0.0...v2.0.0) (2026-11-11)
`,
		},
		// Upstream:
		//   conventional-changelog-writer src/writers.spec.ts: "should sort notes on `text` by default"
		// Upstream checks that the default notesSort sorts notes by text. The
		// preset sets notesSort to compareFunc(), which keeps commit order.
		{
			name: "note order",
			commits: []RawCommit{
				{Hash: fakeHash("1"), Message: "feat(scope): broadcast $destroy event on scope destruction\n\nBREAKING CHANGE: No backward compatibility."},
				{Hash: fakeHash("2"), Message: "fix(ng-list): Allow custom separator\n\nbla bla bla\n\nBREAKING CHANGE: Another change.\nBREAKING CHANGE: Some breaking change."},
			},
			want: `## [2.0.0](https://github.com/owner/repo/compare/v1.0.0...v2.0.0) (2026-11-11)

### ⚠ BREAKING CHANGES

* **scope:** No backward compatibility.
* **ng-list:** Another change.
* **ng-list:** Some breaking change.

### Features

* **scope:** broadcast $destroy event on scope destruction ([1111111](https://github.com/owner/repo/commit/1111111111111111111111111111111111111111))

### Bug Fixes

* **ng-list:** Allow custom separator ([2222222](https://github.com/owner/repo/commit/2222222222222222222222222222222222222222))
`,
		},
		// Upstream:
		//   conventional-changelog-writer src/writers.spec.ts: "should not error if version is not semver"
		// Upstream checks that the version "a.b.c" is written.
		{
			name:    "version not semver",
			version: "a.b.c",
			commits: []RawCommit{
				{Hash: "111", Message: "fix(scope1): First fix"},
				{Hash: "222", Message: "feat(scope2): Second feature"},
			},
			want: `## [a.b.c](https://github.com/owner/repo/compare/v1.0.0...va.b.c) (2026-11-11)

### Features

* **scope2:** Second feature ([222](https://github.com/owner/repo/commit/222))

### Bug Fixes

* **scope1:** First fix ([111](https://github.com/owner/repo/commit/111))
`,
		},
		// Upstream:
		//   conventional-changelog-writer src/template.spec.ts: "should attach a copy of the commit to note"
		// Upstream checks that each note can read the header of its commit.
		// The preset template reads the commit scope; the parser gives only
		// BREAKING CHANGE notes, so the second note replaces the upstream
		// "IMPORTANT CHANGE" note.
		{
			name: "note has commit scope",
			commits: []RawCommit{
				{Hash: fakeHash("1"), Message: "feat: new feature\n\nBREAKING CHANGE: WOW SO MANY CHANGES"},
				{Hash: fakeHash("2"), Message: "chore(first): first commit\n\nBREAKING CHANGE: Not backward compatible.\nBREAKING CHANGE: This is very important!"},
			},
			want: `## [2.0.0](https://github.com/owner/repo/compare/v1.0.0...v2.0.0) (2026-11-11)

### ⚠ BREAKING CHANGES

* WOW SO MANY CHANGES
* **first:** Not backward compatible.
* **first:** This is very important!

### Features

* new feature ([1111111](https://github.com/owner/repo/commit/1111111111111111111111111111111111111111))

### Miscellaneous Chores

* **first:** first commit ([2222222](https://github.com/owner/repo/commit/2222222222222222222222222222222222222222))
`,
		},
		// Upstream:
		//   conventional-changelog-writer src/template.spec.ts: "should not html escape any content"
		// Upstream checks that "`a`" is written as is.
		{
			name: "no HTML escaping",
			commits: []RawCommit{
				{Hash: fakeHash("1"), Message: "feat(<x>): use `a` & <b>\"bold\"</b> 'text'\n\nBREAKING CHANGE: a < b && c > d"},
			},
			want: `## [2.0.0](https://github.com/owner/repo/compare/v1.0.0...v2.0.0) (2026-11-11)

### ⚠ BREAKING CHANGES

* **<x>:** a < b && c > d

### Features

* **<x>:** use ` + "`" + `a` + "`" + ` & <b>"bold"</b> 'text' ([1111111](https://github.com/owner/repo/commit/1111111111111111111111111111111111111111))
`,
		},
		// Upstream:
		//   conventional-changelog-writer src/template.spec.ts: "should ignore a reverted commit"
		// Upstream checks that the reverted commit, its revert and its note
		// are left out, and that the other commits and notes stay. The
		// preset hides the chore.
		{
			name: "reverted commit",
			commits: []RawCommit{
				{Hash: "789d898b5f8422d7f65cc25135af2c1a95a125ac", Message: "revert: feat(): amazing new module\n\nThis reverts commit 56185b7356766d2b30cfa2406b257080272e0b7a."},
				{Hash: "56185b7356766d2b30cfa2406b257080272e0b7a", Message: "feat(): amazing new module\n\nBREAKING CHANGE: some breaking change"},
				{Hash: "815a3f0717bf1dfce007bd076420c609504edcf3", Message: "feat(): new feature\n\nBREAKING CHANGE: WOW SO MANY CHANGES"},
				{Hash: "74a3e4d6d25dee2c0d6483a0a3887417728cbe0a", Message: "chore: first commit"},
			},
			want: `## [2.0.0](https://github.com/owner/repo/compare/v1.0.0...v2.0.0) (2026-11-11)

### ⚠ BREAKING CHANGES

* WOW SO MANY CHANGES

### Features

* new feature ([815a3f0](https://github.com/owner/repo/commit/815a3f0717bf1dfce007bd076420c609504edcf3))
`,
		},
		// Upstream:
		//   conventional-changelog-writer src/context.spec.ts: "should group and sort groups"
		//   conventional-changelog-writer src/context.spec.ts: "should group and sort commits"
		// Upstream checks custom groupBy, commitGroupsSort and commitsSort
		// functions. The preset groups by section, sorts groups by the order
		// of its types and sorts commits by scope, then subject.
		{
			name: "group and commit order",
			commits: []RawCommit{
				{Hash: fakeHash("1"), Message: "revert: feat(x): gone"},
				{Hash: fakeHash("2"), Message: "fix(b): zed"},
				{Hash: fakeHash("3"), Message: "feat(b): alpha"},
				{Hash: fakeHash("4"), Message: "perf: speed"},
				{Hash: fakeHash("5"), Message: "feat: no scope"},
				{Hash: fakeHash("6"), Message: "fix(a): beta"},
				{Hash: fakeHash("7"), Message: "feat(a): zeta"},
				{Hash: fakeHash("8"), Message: "feat(A): upper"},
				{Hash: fakeHash("9"), Message: "feat(a): Alpha"},
			},
			want: `## [2.0.0](https://github.com/owner/repo/compare/v1.0.0...v2.0.0) (2026-11-11)

### Features

* **a:** Alpha ([9999999](https://github.com/owner/repo/commit/9999999999999999999999999999999999999999))
* **A:** upper ([8888888](https://github.com/owner/repo/commit/8888888888888888888888888888888888888888))
* **a:** zeta ([7777777](https://github.com/owner/repo/commit/7777777777777777777777777777777777777777))
* **b:** alpha ([3333333](https://github.com/owner/repo/commit/3333333333333333333333333333333333333333))
* no scope ([5555555](https://github.com/owner/repo/commit/5555555555555555555555555555555555555555))

### Bug Fixes

* **a:** beta ([6666666](https://github.com/owner/repo/commit/6666666666666666666666666666666666666666))
* **b:** zed ([2222222](https://github.com/owner/repo/commit/2222222222222222222222222222222222222222))

### Performance Improvements

* speed ([4444444](https://github.com/owner/repo/commit/4444444444444444444444444444444444444444))

### Reverts

* feat(x): gone ([1111111](https://github.com/owner/repo/commit/1111111111111111111111111111111111111111))
`,
		},
		// Upstream:
		//   conventional-changelog-writer templates/commit.spec.ts: "should link reference from a different repository with an owner"
		//   conventional-changelog-writer templates/commit.spec.ts: "should link reference from a different repository without an owner"
		// Upstream checks [c/d#1](host/c/d/issue/1) for a reference with an
		// owner and for one with a repository only. The parser splits "c/d"
		// into owner and repository; a reference with a repository only
		// uses the owner of the release.
		{
			name: "references to other repositories",
			commits: []RawCommit{
				{Hash: fakeHash("1"), Message: "fix: link refs\n\ncloses c/d#1, d#2, c/d/e#3"},
			},
			want: `## [2.0.0](https://github.com/owner/repo/compare/v1.0.0...v2.0.0) (2026-11-11)

### Bug Fixes

* link refs ([1111111](https://github.com/owner/repo/commit/1111111111111111111111111111111111111111)), closes [c/d#1](https://github.com/c/d/issues/1) [d#2](https://github.com/owner/d/issues/2) [c/d/e#3](https://github.com/c/d/e/issues/3)
`,
		},
	}
	runUpstreamNotesCases(t, cases)
}
