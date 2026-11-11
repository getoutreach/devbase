// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Ports of the upstream tests of
// @semantic-release/commit-analyzer 13.0.1 and semantic-release 25.0.9
// that apply to the release type, the version calculation, the last
// release, tags and channels, and the branch rules of the Go port. Cases
// that upstream runs with another preset use the conventionalcommits
// preset here, with the expected values that commit-analyzer 13.0.1
// returned for these inputs.

package release

import (
	"strconv"
	"testing"

	"github.com/getoutreach/devbase/v2/internal/release/releaserc"
	"github.com/getoutreach/devbase/v2/internal/release/spec"
	"gotest.tools/v3/assert"
)

// TestUpstreamCompareReleaseTypes ports commit-analyzer
// test/compare-release-types.test.js. compareReleaseTypes(current, next)
// is outranks(next, current), and "false" stands for false and null.
func TestUpstreamCompareReleaseTypes(t *testing.T) {
	// All cases are from "Compares release types". null is "false" in Go,
	// so the upstream null rows are the same as the "false" rows.
	tests := []struct {
		current, next spec.Bump
		want          bool
	}{
		{"patch", "minor", true},
		{"patch", "major", true},
		{"minor", "major", true},
		{"major", "major", false},
		{"major", "minor", false},
		{"major", "patch", false},
		{"minor", "patch", false},
		{"major", spec.BumpNoRelease, true},
		{"minor", spec.BumpNoRelease, true},
		{"patch", spec.BumpNoRelease, true},
	}
	for _, tt := range tests {
		assert.Equal(t, outranks(tt.next, tt.current), tt.want, "compareReleaseTypes(%q, %q)", tt.current, tt.next)
	}
}

// TestUpstreamAnalyzeCommit ports commit-analyzer
// test/analyze-commit.test.js. analyzeCommit returns undefined when no
// rule matches, which is matched == false in Go.
func TestUpstreamAnalyzeCommit(t *testing.T) {
	breaking := []Note{{Title: "BREAKING CHANGE", Text: "some breaking change"}}
	revert := &Revert{Header: "Update: First feature", Hash: "123"}
	globRules := []spec.Rule{{Type: "docs", Scope: "b*", Release: "minor"}}
	tests := []struct {
		name    string
		rules   []spec.Rule
		commit  Commit
		want    spec.Bump
		matched bool
	}{
		{
			name:    "analyze-commit.test.js: Match breaking change",
			rules:   []spec.Rule{{Breaking: true, Release: "major"}},
			commit:  Commit{Notes: breaking},
			want:    spec.BumpMajor,
			matched: true,
		},
		{
			name:    "analyze-commit.test.js: Match revert commit",
			rules:   []spec.Rule{{Revert: true, Release: "patch"}},
			commit:  Commit{Revert: revert},
			want:    spec.BumpPatch,
			matched: true,
		},
		{
			name:    "analyze-commit.test.js: Match multiple criteria with breaking change",
			rules:   []spec.Rule{{Type: "feat", Breaking: true, Release: "major"}},
			commit:  Commit{Type: "feat", Notes: breaking},
			want:    spec.BumpMajor,
			matched: true,
		},
		{
			name:    "analyze-commit.test.js: Match multiple criteria with revert",
			rules:   []spec.Rule{{Type: "feat", Revert: true, Release: "major"}},
			commit:  Commit{Type: "feat", Revert: revert},
			want:    spec.BumpMajor,
			matched: true,
		},
		{
			// Upstream uses the number 1 as the scope; Go scopes are strings.
			name:    "analyze-commit.test.js: Match multiple criteria",
			rules:   []spec.Rule{{Type: "feat", Scope: "1", Release: "major"}},
			commit:  Commit{Type: "feat", Scope: "1"},
			want:    spec.BumpMajor,
			matched: true,
		},
		{
			name: "analyze-commit.test.js: Match only if all criteria are verified",
			rules: []spec.Rule{
				{Type: "fix", Release: "minor"},
				{Type: "fix", Breaking: true, Release: "major"},
			},
			commit:  Commit{Type: "fix", Notes: breaking},
			want:    spec.BumpMajor,
			matched: true,
		},
		{
			name:   "analyze-commit.test.js: Return undefined if there is no match",
			rules:  []spec.Rule{{Type: "feat", Breaking: true, Release: "major"}},
			commit: Commit{Type: "fix", Notes: breaking},
		},
		{
			// The null type is the empty string in Go.
			name:   "analyze-commit.test.js: Return undefined for commit with falsy properties",
			rules:  []spec.Rule{{Type: "feat"}},
			commit: Commit{},
		},
		{
			name:    "analyze-commit.test.js: Match with glob (bar)",
			rules:   globRules,
			commit:  Commit{Type: "docs", Scope: "bar"},
			want:    spec.BumpMinor,
			matched: true,
		},
		{
			name:    "analyze-commit.test.js: Match with glob (baz)",
			rules:   globRules,
			commit:  Commit{Type: "docs", Scope: "baz"},
			want:    spec.BumpMinor,
			matched: true,
		},
		{
			name:   "analyze-commit.test.js: Match with glob (foo)",
			rules:  globRules,
			commit: Commit{Type: "docs", Scope: "foo"},
		},
		{
			name: "analyze-commit.test.js: Return highest release type if multiple rules match",
			rules: []spec.Rule{
				{Type: "feat", Release: "minor"},
				{Breaking: true, Release: "minor"},
				{Type: "feat", Breaking: true, Release: "major"},
			},
			commit:  Commit{Type: "feat", Notes: breaking},
			want:    spec.BumpMajor,
			matched: true,
		},
		{
			// This case also stands for `Return "null" for release type if the
			// matching rule has "release" set to "null"`: "false" stands for
			// null in matchRules, and releaserc.Parse rejects release: null.
			name:    `analyze-commit.test.js: Return "false" for release type if the matching rule has "release" set to "false"`,
			rules:   []spec.Rule{{Type: "fix", Release: spec.BumpNoRelease}},
			commit:  Commit{Type: "fix"},
			matched: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, matched := matchRules(&tt.commit, tt.rules)
			assert.Equal(t, got, tt.want)
			assert.Equal(t, matched, tt.matched)
		})
	}
}

// TestUpstreamAnalyzeCommits ports commit-analyzer
// test/integration.test.js. Cases marked "adapted" use another preset
// upstream; their inputs are rewritten for conventionalcommits and the
// expected values come from commit-analyzer 13.0.1 with that preset. The
// commit of message i has the hash fakeHash(i+1).
func TestUpstreamAnalyzeCommits(t *testing.T) {
	tests := []struct {
		name     string
		messages []string
		rules    []spec.Rule
		want     spec.Bump
	}{
		{
			name:     `integration.test.js: Parse with "conventional-changelog-angular" by default`,
			messages: []string{"fix(scope1): First fix", "feat(scope2): Second feature"},
			want:     spec.BumpMinor,
		},
		{
			name: "integration.test.js: Exclude commits if they have a matching revert commits",
			messages: []string{
				"fix(scope): First fix",
				"revert: feat(scope): First feature\n\nThis reverts commit " + fakeHash("3") + ".\n",
				"feat(scope): First feature",
			},
			want: spec.BumpPatch,
		},
		{
			// Adapted: the module's rules are inline.
			name:     `integration.test.js: Accept a "releaseRules" option that reference a requirable module`,
			messages: []string{"fix(scope1): First fix", "feat(scope2): Second feature"},
			rules: []spec.Rule{
				{Breaking: true, Release: "major"},
				{Type: "feat", Release: "minor"},
				{Type: "fix", Release: "patch"},
				{Type: "perf", Release: "patch"},
			},
			want: spec.BumpMinor,
		},
		{
			// Adapted: the eslint messages are parsed with conventionalcommits.
			name: `integration.test.js: Return "major" if there is a breaking change, using default releaseRules`,
			messages: []string{
				"Fix: First fix (fixes #123)",
				"Update: Second feature (fixes #456) \n\n BREAKING CHANGE: break something",
			},
			want: spec.BumpMajor,
		},
		{
			name:     `integration.test.js: Return "major" if there is a "conventionalcommits" breaking change, using default releaseRules`,
			messages: []string{"fix: First fix", "feat!: Breaking change feature"},
			want:     spec.BumpMajor,
		},
		{
			name:     `integration.test.js: Return "patch" if there is only types set to "patch", using default releaseRules`,
			messages: []string{"fix: First fix (fixes #123)", "perf: perf improvement"},
			want:     spec.BumpPatch,
		},
		{
			// Adapted: the tag rule is a type rule and the message glob is
			// a subject glob.
			name:     `integration.test.js: Allow to use glob in "releaseRules" configuration`,
			messages: []string{"chore: First chore (fixes #123)", "docs: update README (fixes #456)"},
			rules: []spec.Rule{
				{Type: "chore", Release: "patch"},
				{Subject: "*README*", Release: "minor"},
			},
			want: spec.BumpMinor,
		},
		{
			name:     `integration.test.js: Return "null" if no rule match`,
			messages: []string{"doc: doc update", "chore: Chore"},
			want:     spec.BumpNone,
		},
		{
			// Adapted: the tag rules are type rules.
			name:     "integration.test.js: Process rules in order and apply highest match",
			messages: []string{"chore: First chore (fixes #123)", "docs: update README (fixes #456)"},
			rules: []spec.Rule{
				{Type: "chore", Release: "minor"},
				{Type: "chore", Release: "patch"},
			},
			want: spec.BumpMinor,
		},
		{
			// Adapted: the tag rule is a type rule.
			name: "integration.test.js: Process rules in order and apply highest match from config even if default has an higher match",
			messages: []string{
				"chore: First chore (fixes #123)",
				"docs: update README (fixes #456) \n\n BREAKING CHANGE: break something",
			},
			rules: []spec.Rule{
				{Type: "chore", Release: "patch"},
				{Breaking: true, Release: "minor"},
			},
			want: spec.BumpMinor,
		},
		{
			name:     `integration.test.js: Allow to overwrite default "releaseRules" with "false"`,
			messages: []string{"chore: First chore", "feat: new feature"},
			rules:    []spec.Rule{{Type: "feat", Release: spec.BumpNoRelease}},
			want:     spec.BumpNone,
		},
		{
			name:     `integration.test.js: Commits with an associated custom release type have higher priority than commits with release "false"`,
			messages: []string{"feat: Feature to skip", "docs: update README"},
			rules: []spec.Rule{
				{Type: "feat", Release: spec.BumpNoRelease},
				{Type: "docs", Release: "patch"},
			},
			want: spec.BumpPatch,
		},
		{
			name:     `integration.test.js: Commits with an associated default release type have higher priority than commits with release "false"`,
			messages: []string{"feat: new feature", "fix: new Fix"},
			rules:    []spec.Rule{{Type: "feat", Release: spec.BumpNoRelease}},
			want:     spec.BumpPatch,
		},
		{
			// Adapted: the eslint messages and tag rule are conventionalcommits.
			name:     `integration.test.js: Use default "releaseRules" if none of provided match`,
			messages: []string{"chore: First chore", "feat: new feature"},
			rules:    []spec.Rule{{Type: "chore", Release: "patch"}},
			want:     spec.BumpMinor,
		},
		{
			name:     "integration.test.js: Filter out empty commits",
			messages: []string{"", "fix(scope1): First fix"},
			want:     spec.BumpPatch,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raws := make([]RawCommit, len(tt.messages))
			for i, m := range tt.messages {
				raws[i] = RawCommit{Hash: fakeHash(strconv.Itoa(i + 1)), Message: m}
			}
			assert.Equal(t, Analyze(ParseCommits(raws), tt.rules), tt.want)
		})
	}
}

// TestUpstreamGlobParity checks spec.MatchGlob against micromatch 4.0.8
// isMatch, which lib/analyze-commit.js calls for string rule fields. The
// first case is the glob case of analyze-commit.test.js; the others are
// micromatch results for patterns that path.Match could read in another
// way. Syntax that path.Match does not have is rejected by
// releaserc.Parse, so it fails closed instead of matching differently;
// TestParseFailsClosed in package releaserc checks that.
func TestUpstreamGlobParity(t *testing.T) {
	tests := []struct {
		pattern, value string
		want           bool
	}{
		{"b*", "bar", true},
		{"core-*", "core-db", true},
		{"*README*", "update README (fixes #456)", true},
		{"*", "a/b", false},
		{"*/x", "a/x", true},
		{"[^a]", "b", true},
		{"[!a]", "b", false},
		{`\*`, "*", true},
		{"A", "a", false},
		{"[a-", "a", false},
		// micromatch does not match an empty string, and a null field
		// never reaches micromatch.
		{"*", "", false},
		// Without the dot option, "*" and "?" do not match a leading dot.
		{"*", ".hidden", false},
		{"?x", ".x", false},
		{"x/*", "x/.b", false},
		{".*", ".x", true},
		{"[.]x", ".x", true},
	}
	for _, tt := range tests {
		t.Run(tt.pattern+" "+tt.value, func(t *testing.T) {
			assert.Equal(t, spec.MatchGlob(tt.pattern, tt.value), tt.want)
		})
	}

	// A rule with a "*" scope does not match a commit without a scope
	// upstream, so the default rules give a minor release.
	t.Run("scope glob on a commit without a scope", func(t *testing.T) {
		commits := ParseCommits([]RawCommit{{Hash: "h", Message: "feat: x"}})
		assert.Equal(t, Analyze(commits, []spec.Rule{{Scope: "*", Release: "major"}}), spec.BumpMinor)
	})
}

// TestUpstreamSkipRelease ports semantic-release test/integration.test.js
// "Exclude commits with [skip release] or [release skip] from analysis",
// which tests the analyzeCommits preprocess of lib/definitions/plugins.js.
func TestUpstreamSkipRelease(t *testing.T) {
	// The messages are newest first, as git log returns them. The first
	// commit is the one that gitRepo(true) makes.
	messages := []string{
		"Test commit\n\n commit body\n[release skip]",
		"Test commit\n\n commit body\n[skip release]",
		"Test commit [skip    release]",
		"Test commit [Skip Release]",
		"Test commit [Release Skip]",
		"Test commit [release skip]",
		"Test commit [skip release]",
		"Test commit",
		"Initial commit",
	}
	raws := make([]RawCommit, len(messages))
	for i, m := range messages {
		raws[i] = RawCommit{Hash: fakeHash(strconv.Itoa(i + 1)), Message: m}
	}
	got := DropSkipped(ParseCommits(raws))
	assert.Equal(t, len(got), 2)
	assert.Equal(t, got[0].Message, "Test commit")
	assert.Equal(t, got[1].Message, "Initial commit")
}

// upstreamTag returns the tag v<version> with the channels of its git
// note. The empty string stands for the null channel; no channels stand
// for a tag object without channels.
func upstreamTag(t *testing.T, version string, channels ...string) Tag {
	t.Helper()
	v := mustVersion(t, version)
	return Tag{Name: "v" + v.String(), Version: v, Channels: channels}
}

// upstreamLast returns a pointer to a tag, for the lastRelease of a case.
func upstreamLast(t *testing.T, version string, channels ...string) *Tag {
	t.Helper()
	tag := upstreamTag(t, version, channels...)
	return &tag
}

// TestUpstreamNextVersion ports semantic-release
// test/get-next-version.test.js. The branch channel is the
// nextRelease.channel of the case, and the tags are branch.tags.
func TestUpstreamNextVersion(t *testing.T) {
	master := spec.Branch{Name: "master"}
	beta := spec.Branch{Name: "beta", Prerelease: "beta", Channel: "beta"}
	alpha := spec.Branch{Name: "alpha", Prerelease: "alpha", Channel: "alpha"}
	// betaNoChannel is the beta branch of the cases without a
	// nextRelease.channel.
	betaNoChannel := spec.Branch{Name: "beta", Prerelease: "beta"}
	v100 := upstreamTag(t, "1.0.0", "")
	tests := []struct {
		name   string
		branch spec.Branch
		tags   []Tag
		last   *Tag
		bump   spec.Bump
		want   string
	}{
		{"Increase version for patch release", master, []Tag{v100}, upstreamLast(t, "1.0.0", ""), spec.BumpPatch, "1.0.1"},
		{"Increase version for minor release", master, []Tag{v100}, upstreamLast(t, "1.0.0", ""), spec.BumpMinor, "1.1.0"},
		{"Increase version for major release", master, []Tag{v100}, upstreamLast(t, "1.0.0", ""), spec.BumpMajor, "2.0.0"},
		{"Return 1.0.0 if there is no previous release", master, nil, nil, spec.BumpMinor, "1.0.0"},
		{
			"Increase version for patch release on prerelease branch (1)", beta,
			[]Tag{v100},
			upstreamLast(t, "1.0.0", ""), spec.BumpPatch, "1.0.1-beta.1",
		},
		{
			"Increase version for patch release on prerelease branch (2)", beta,
			[]Tag{v100, upstreamTag(t, "1.0.1-beta.1", "beta")},
			upstreamLast(t, "1.0.1-beta.1", "beta"), spec.BumpPatch, "1.0.1-beta.2",
		},
		{
			"Increase version for patch release on prerelease branch (3)", alpha,
			[]Tag{upstreamTag(t, "1.0.1-beta.1", "beta")},
			upstreamLast(t, "1.0.1-beta.1", "beta"), spec.BumpPatch, "1.0.2-alpha.1",
		},
		{
			"Increase version for minor release on prerelease branch (1)", beta,
			[]Tag{v100},
			upstreamLast(t, "1.0.0", ""), spec.BumpMinor, "1.1.0-beta.1",
		},
		{
			"Increase version for minor release on prerelease branch (2)", beta,
			[]Tag{v100, upstreamTag(t, "1.1.0-beta.1", "beta")},
			upstreamLast(t, "1.1.0-beta.1", "beta"), spec.BumpMinor, "1.1.0-beta.2",
		},
		{
			"Increase version for minor release on prerelease branch (3)", alpha,
			[]Tag{upstreamTag(t, "1.1.0-beta.1", "beta")},
			upstreamLast(t, "1.1.0-beta.1", "beta"), spec.BumpMinor, "1.2.0-alpha.1",
		},
		{
			"Increase version for major release on prerelease branch (1)", beta,
			[]Tag{v100},
			upstreamLast(t, "1.0.0", ""), spec.BumpMajor, "2.0.0-beta.1",
		},
		{
			"Increase version for major release on prerelease branch (2)", beta,
			[]Tag{v100, upstreamTag(t, "2.0.0-beta.1", "beta")},
			upstreamLast(t, "2.0.0-beta.1", "beta"), spec.BumpMajor, "2.0.0-beta.2",
		},
		{
			"Increase version for major release on prerelease branch (3)", alpha,
			[]Tag{upstreamTag(t, "2.0.0-beta.1", "beta")},
			upstreamLast(t, "2.0.0-beta.1", "beta"), spec.BumpMajor, "3.0.0-alpha.1",
		},
		{"Return 1.0.0 if there is no previous release on prerelease branch", betaNoChannel, nil, nil, spec.BumpMinor, "1.0.0-beta.1"},
		{
			"Increase version for release on prerelease branch after previous commits were merged to release branch", betaNoChannel,
			[]Tag{v100, upstreamTag(t, "1.1.0", ""), upstreamTag(t, "1.1.0-beta.1", "", "beta")},
			upstreamLast(t, "1.1.0", ""), spec.BumpMinor, "1.2.0-beta.1",
		},
		{
			"Increase version for release on prerelease branch based on highest commit type since last regular release", betaNoChannel,
			[]Tag{v100, upstreamTag(t, "1.1.0-beta.1", "", "beta")},
			upstreamLast(t, "v1.1.0-beta.1", ""), spec.BumpMajor, "2.0.0-beta.1",
		},
		{
			"Increase version for release on prerelease branch when there is no regular releases on other branches", beta,
			[]Tag{upstreamTag(t, "1.0.0-beta.1", "beta")},
			upstreamLast(t, "v1.0.0-beta.1", "beta"), spec.BumpMinor, "1.0.0-beta.2",
		},
		{
			"Increase patch when previous version shares HEAD with other releases", alpha,
			[]Tag{
				upstreamTag(t, "1.0.0-beta.1", "beta"),
				upstreamTag(t, "1.0.0-beta.2", "alpha", "beta"),
				upstreamTag(t, "1.0.0-alpha.1", "alpha", "beta"),
			},
			upstreamLast(t, "v1.0.0-alpha.1", "alpha", "beta"), spec.BumpPatch, "1.0.0-alpha.2",
		},
	}
	for _, tt := range tests {
		t.Run("get-next-version.test.js: "+tt.name, func(t *testing.T) {
			assert.Equal(t, NextVersion(&tt.branch, tt.last, tt.tags, tt.bump).String(), tt.want)
		})
	}
}

// TestUpstreamLastRelease ports semantic-release
// test/get-last-release.test.js, and the getLatestVersion cases of
// test/utils.test.js without prereleases, which LastRelease does on a
// release branch. want is the tag name, or empty for no release.
func TestUpstreamLastRelease(t *testing.T) {
	master := spec.Branch{Name: "master"}
	latest := []Tag{
		upstreamTag(t, "1.2.3-alpha.3", ""),
		upstreamTag(t, "1.2.0", ""),
		upstreamTag(t, "1.0.1", ""),
		upstreamTag(t, "1.0.0-alpha.1", ""),
	}
	tests := []struct {
		name         string
		branch       spec.Branch
		tags         []Tag
		want         string
		wantChannels []string
	}{
		{
			name:   "get-last-release.test.js: Get the highest non-prerelease valid tag",
			branch: master,
			tags:   []Tag{upstreamTag(t, "2.0.0"), upstreamTag(t, "1.0.0"), upstreamTag(t, "3.0.0-beta.1")},
			want:   "v2.0.0",
		},
		{
			name:   "get-last-release.test.js: Get the highest prerelease valid tag, ignoring other tags from other prerelease channels",
			branch: spec.Branch{Name: "beta", Prerelease: "beta", Channel: "beta"},
			tags: []Tag{
				upstreamTag(t, "1.0.0-beta.1", "beta"),
				upstreamTag(t, "1.0.0-beta.2", "beta"),
				upstreamTag(t, "1.0.0-alpha.1", "alpha"),
			},
			want:         "v1.0.0-beta.2",
			wantChannels: []string{"beta"},
		},
		{
			// Upstream has a second test with this title that uses
			// prerelease: true. releaserc.Parse turns prerelease: true into
			// the branch name, so that test gives the same branch.
			name:   "get-last-release.test.js: Get the correct prerelease tag, when other prereleases share the same git HEAD",
			branch: spec.Branch{Name: "alpha", Prerelease: "alpha", Channel: "alpha"},
			tags: []Tag{
				upstreamTag(t, "1.0.0-beta.1", "beta"),
				upstreamTag(t, "1.0.0-beta.2", "alpha", "beta"),
				upstreamTag(t, "1.0.0-alpha.1", "alpha", "beta"),
			},
			want:         "v1.0.0-alpha.1",
			wantChannels: []string{"alpha", "beta"},
		},
		{
			name:   "get-last-release.test.js: Return empty object if no valid tag is found",
			branch: master,
			tags:   []Tag{upstreamTag(t, "3.0.0-beta.1")},
		},
		{
			name:         "utils.test.js: getLatestVersion (1)",
			branch:       master,
			tags:         latest,
			want:         "v1.2.0",
			wantChannels: []string{""},
		},
		{
			name:   "utils.test.js: getLatestVersion (2)",
			branch: master,
			tags:   []Tag{upstreamTag(t, "1.2.3-alpha.3", ""), upstreamTag(t, "1.2.3-alpha.2", "")},
		},
		{
			name:   "utils.test.js: getLatestVersion (3)",
			branch: master,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := LastRelease(&tt.branch, tt.tags)
			if tt.want == "" {
				assert.Assert(t, got == nil, "got %+v", got)
				return
			}
			assert.Assert(t, got != nil)
			assert.Equal(t, got.Name, tt.want)
			assert.DeepEqual(t, got.Channels, tt.wantChannels)
		})
	}
}

// tagView is the part of a Tag that the get-tags cases compare.
type tagView struct {
	Name     string
	Version  string
	Channels []string
}

// viewTags returns the tagView of each tag, or nil for no tags.
func viewTags(tags []Tag) []tagView {
	if len(tags) == 0 {
		return nil
	}
	out := make([]tagView, 0, len(tags))
	for _, t := range tags {
		out = append(out, tagView{Name: t.Name, Version: t.Version.String(), Channels: t.Channels})
	}
	return out
}

// upstreamCommits makes one empty commit for each message and returns
// them newest first, as the gitCommits helper of semantic-release does.
func upstreamCommits(f *fixture, messages ...string) []RawCommit {
	f.t.Helper()
	var out []RawCommit
	for _, m := range messages {
		f.commit(m)
		out = append([]RawCommit{{Hash: f.run("rev-parse", "HEAD"), Message: m}}, out...)
	}
	return out
}

// TestUpstreamGetTags ports semantic-release
// test/branches/get-tags.test.js. The Go port has the tag format
// v${version} only, so the cases with another tagFormat use it.
func TestUpstreamGetTags(t *testing.T) {
	t.Run("get-tags.test.js: Get the valid tags", func(t *testing.T) {
		f := newRepo(t, "master")
		f.commit("First")
		f.run("tag", "foo")
		f.run("tag", "v2.0.0")
		f.commit("Second")
		f.run("tag", "v1.0.0")
		f.commit("Third")
		f.run("tag", "v3.0")
		f.commit("Fourth")
		f.run("tag", "v3.0.0-beta.1")

		tags, err := f.git.Tags(t.Context(), "master")
		assert.NilError(t, err)
		assert.DeepEqual(t, viewTags(tags), []tagView{
			{"v1.0.0", "1.0.0", []string{""}},
			{"v2.0.0", "2.0.0", []string{""}},
			{"v3.0.0-beta.1", "3.0.0-beta.1", []string{""}},
		})
	})

	t.Run("get-tags.test.js: Get the valid tags from multiple branches", func(t *testing.T) {
		f := newRepo(t, "master")
		f.commit("First")
		f.run("tag", "v1.0.0")
		f.addNote("v1.0.0", `{"channels":[null,"1.x"]}`)
		f.commit("Second")
		f.run("tag", "v1.1.0")
		f.addNote("v1.1.0", `{"channels":[null,"1.x"]}`)
		f.run("checkout", "-q", "-b", "1.x")
		f.run("checkout", "-q", "master")
		f.commit("Third")
		f.run("tag", "v2.0.0")
		f.addNote("v2.0.0", `{"channels":[null,"next"]}`)
		f.run("checkout", "-q", "-b", "next")
		f.commit("Fourth")
		f.run("tag", "v3.0.0")
		f.addNote("v3.0.0", `{"channels":["next"]}`)

		want := []tagView{
			{"v1.0.0", "1.0.0", []string{"", "1.x"}},
			{"v1.1.0", "1.1.0", []string{"", "1.x"}},
		}
		for _, branch := range []string{"1.x", "master", "next"} {
			switch branch {
			case "master":
				want = append(want, tagView{"v2.0.0", "2.0.0", []string{"", "next"}})
			case "next":
				want = append(want, tagView{"v3.0.0", "3.0.0", []string{"next"}})
			}
			tags, err := f.git.Tags(t.Context(), branch)
			assert.NilError(t, err)
			assert.DeepEqual(t, viewTags(tags), want)
		}
	})

	// Adapted: upstream uses the prefix@v${version} format, which none
	// of these tags match. With v${version}, none of them is valid.
	t.Run("get-tags.test.js: Return branches with and empty tags array if no valid tag is found", func(t *testing.T) {
		f := newRepo(t, "master")
		f.commit("First")
		f.run("tag", "foo")
		f.commit("Second")
		f.run("tag", "v2.0.x")
		f.commit("Third")
		f.run("tag", "v3.0")

		tags, err := f.git.Tags(t.Context(), "master")
		assert.NilError(t, err)
		assert.Equal(t, len(tags), 0)
	})

	// Adapted: with the v${version} format the tags on next are valid,
	// so only the master half of the case applies.
	t.Run("get-tags.test.js: Return branches with and empty tags array if no valid tag is found in history of configured branches",
		func(t *testing.T) {
			f := newRepo(t, "master")
			f.commit("First")
			f.run("checkout", "-q", "-b", "next")
			for _, tag := range []string{"v1.0.0", "v2.0.0", "v3.0.0"} {
				f.commit("Commit " + tag)
				f.run("tag", tag)
				f.addNote(tag, `{"channels":[null,"next"]}`)
			}
			f.run("checkout", "-q", "master")

			tags, err := f.git.Tags(t.Context(), "master")
			assert.NilError(t, err)
			assert.Equal(t, len(tags), 0)
		})
}

// TestUpstreamTagNotes ports the semantic-release tests for the git
// notes that tagChannels reads: getTagsNotes in test/git.test.js, and
// extractGitLogTags in test/utils.test.js.
func TestUpstreamTagNotes(t *testing.T) {
	// get-tags.js gives a tag without a usable note the [null] channels.
	for _, tt := range []struct{ name, note string }{
		{"git.test.js: Return undefined if there is no commit note", ""},
		{"git.test.js: Return undefined if a commit note is invalid", "non-json note"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := newRepo(t, "master")
			f.commit("First")
			f.run("tag", "v1.0.0")
			if tt.note != "" {
				f.addNote("v1.0.0", tt.note)
			}
			tags, err := f.git.Tags(t.Context(), "master")
			assert.NilError(t, err)
			assert.DeepEqual(t, viewTags(tags), []tagView{{"v1.0.0", "1.0.0", []string{""}}})
		})
	}

	// Both tags of "(tag: v1.2.3, tag: 5833/merge)" get the note.
	t.Run("utils.test.js: extractGitLogTags", func(t *testing.T) {
		f := newRepo(t, "master")
		f.commit("First")
		f.run("tag", "v1.2.3")
		f.run("tag", "5833/merge")
		f.addNote("v1.2.3", `{"channels":["next"]}`)
		channels, err := f.git.tagChannels(t.Context())
		assert.NilError(t, err)
		assert.DeepEqual(t, channels, map[string][]string{"v1.2.3": {"next"}, "5833/merge": {"next"}})
	})
}

// TestUpstreamGetCommits ports semantic-release test/get-commits.test.js
// for Git.Commits. lastRelease.gitHead is from, and nextRelease.gitHead
// is to (HEAD when it is not set).
func TestUpstreamGetCommits(t *testing.T) {
	t.Run("get-commits.test.js: Get all commits when there is no last release", func(t *testing.T) {
		f := newRepo(t, "master")
		commits := upstreamCommits(f, "First", "Second")
		got, err := f.git.Commits(t.Context(), "", "HEAD")
		assert.NilError(t, err)
		assert.DeepEqual(t, got, commits)
	})

	t.Run("get-commits.test.js: Get all commits since gitHead (from lastRelease)", func(t *testing.T) {
		f := newRepo(t, "master")
		commits := upstreamCommits(f, "First", "Second", "Third")
		got, err := f.git.Commits(t.Context(), commits[2].Hash, "HEAD")
		assert.NilError(t, err)
		assert.DeepEqual(t, got, commits[:2])
	})

	t.Run("get-commits.test.js: Get all commits since gitHead (from lastRelease) on a detached head repo", func(t *testing.T) {
		f := newRepo(t, "master")
		commits := upstreamCommits(f, "First", "Second", "Third")
		f.run("checkout", "-q", "--detach", commits[1].Hash)
		got, err := f.git.Commits(t.Context(), commits[2].Hash, "HEAD")
		assert.NilError(t, err)
		assert.DeepEqual(t, got, commits[1:2])
	})

	t.Run("get-commits.test.js: Get all commits between lastRelease.gitHead and a shas", func(t *testing.T) {
		f := newRepo(t, "master")
		commits := upstreamCommits(f, "First", "Second", "Third")
		got, err := f.git.Commits(t.Context(), commits[2].Hash, commits[1].Hash)
		assert.NilError(t, err)
		assert.DeepEqual(t, got, commits[1:2])
	})

	t.Run("get-commits.test.js: Return empty array if lastRelease.gitHead is the last commit", func(t *testing.T) {
		f := newRepo(t, "master")
		commits := upstreamCommits(f, "First", "Second")
		got, err := f.git.Commits(t.Context(), commits[0].Hash, "HEAD")
		assert.NilError(t, err)
		assert.Equal(t, len(got), 0)
	})

	// Upstream returns no commits for a repository without commits. The
	// Go planner stops earlier, when it reads the HEAD commit, so
	// Git.Commits returns the git error.
	t.Run("get-commits.test.js: Return empty array if there is no commits", func(t *testing.T) {
		f := newRepo(t, "master")
		_, err := f.git.Commits(t.Context(), "", "HEAD")
		assert.ErrorContains(t, err, "HEAD")
	})
}

// TestUpstreamNormalizeBranches ports the channel and prerelease parts of
// semantic-release test/branches/normalize.test.js, through
// releaserc.Parse. The Go port has no channel key, so the explicit
// channel of upstream is left out where it is the default.
func TestUpstreamNormalizeBranches(t *testing.T) {
	t.Run("normalize.test.js: Prerelease branches", func(t *testing.T) {
		// Parse needs a release branch, so master is added.
		cfg, _, err := releaserc.Parse([]byte("preset: conventionalcommits\n" +
			"branches: [master, {name: beta, prerelease: true}, {name: alpha, prerelease: preview}]\n"))
		assert.NilError(t, err)
		assert.DeepEqual(t, cfg.Branches, []spec.Branch{
			{Name: "master"},
			{Name: "beta", Prerelease: "beta", Channel: "beta"},
			{Name: "alpha", Prerelease: "preview", Channel: "alpha"},
		})
	})
}
