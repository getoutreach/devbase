// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Tests for the .releaserc.yaml loader.

package releaserc

import (
	"strconv"
	"testing"

	"gotest.tools/v3/assert"
)

// prereleaseConfig is the stencil output for a CLI repo with
// prereleases, a major-version check and an orb.
const prereleaseConfig = `
preset: conventionalcommits
branches:
  - name: main
    prerelease: "rc"
  - name: release
plugins:
  - - "@semantic-release/commit-analyzer"
    - releaseRules:
        - breaking: true
          release: minor
        - type: revert
          release: patch
  - - "@semantic-release/exec"
    - generateNotesCmd: |-
        ./scripts/shell-wrapper.sh major-release-checker.js ${lastRelease.version} ${nextRelease.version}
  - - "@semantic-release/exec"
    - generateNotesCmd: "make release APP_VERSION=${nextRelease.version}"
  - "@semantic-release/release-notes-generator"
  - - "@semantic-release/github"
    - assets:
        - "dist/*.tar.gz"
        - "dist/checksums.txt"
  - - "@getoutreach/semantic-release-circleci-orb"
    - orbName: getoutreach/shared
      orbPath: orb.yml
`

func TestParse(t *testing.T) {
	cfg, err := Parse([]byte(prereleaseConfig))
	assert.NilError(t, err)

	assert.DeepEqual(t, cfg.Branches, []Branch{
		{Name: "main", Prerelease: "rc", Channel: "main"},
		{Name: "release", Channel: ""},
	})
	assert.DeepEqual(t, cfg.ReleaseRules, []Rule{
		{Breaking: true, Release: "minor"},
		{Type: "revert", Release: "patch"},
	})
	assert.Assert(t, cfg.MajorCheck, "major-release-checker.js becomes the built-in check")
	assert.Equal(t, len(cfg.Warnings), 0)
}

// analyzerConfig returns a config with one commit-analyzer plugin whose
// options are the YAML mapping opts.
func analyzerConfig(opts string) string {
	return "preset: conventionalcommits\nbranches: [master]\nplugins:\n" +
		"  - - \"@semantic-release/commit-analyzer\"\n    - " + opts + "\n"
}

// branchesConfig returns a config with the YAML branches list branches.
func branchesConfig(branches string) string {
	return "preset: conventionalcommits\nbranches: " + branches + "\n"
}

// TestParseFailsClosed checks that Parse rejects a configuration that the
// release command cannot run as semantic-release does.
func TestParseFailsClosed(t *testing.T) {
	tests := map[string]string{
		"unknown plugin": `
preset: conventionalcommits
branches: [main]
plugins: ["@semantic-release/changelog"]`,
		"unknown plugin option": `
preset: conventionalcommits
branches: [main]
plugins: [["@semantic-release/github", {draftRelease: true}]]`,
		"unknown top-level key": `
preset: conventionalcommits
tagFormat: "pkg/v${version}"
branches: [main]`,
		"other preset": `
preset: angular
branches: [main]`,
		"empty branch name": `
preset: conventionalcommits
branches: [{name: }]`,
		"invalid prerelease identifier": `
preset: conventionalcommits
branches: [{name: release/next, prerelease: true}, main]`,
		"branch channel": `
preset: conventionalcommits
branches: [{name: main, channel: next}]`,
		"unknown template field": `
preset: conventionalcommits
branches: [main]
plugins: [["@semantic-release/exec", {publishCmd: "echo ${nextRelease.notes}"}]]`,
		"lodash template": `
preset: conventionalcommits
branches: [main]
plugins: [["@semantic-release/exec", {publishCmd: "echo <%= nextRelease.version %>"}]]`,
	}
	// Upstream tests that expect a configuration error, from commit-analyzer
	// 13.0.1 and semantic-release 25.0.9.
	for _, tt := range []struct{ name, yaml string }{
		{
			name: "integration.test.js: Throw error if \"preset\" doesn`t exist",
			yaml: "preset: unknown-preset\nbranches: [master]\n",
		},
		{
			name: `integration.test.js: Throw error if "releaseRules" is not an Array or a String`,
			yaml: analyzerConfig("releaseRules: {}"),
		},
		{
			name: "integration.test.js: Throw error if \"config\" doesn`t exist",
			yaml: analyzerConfig("config: unknown-config"),
		},
		{
			name: `integration.test.js: Throw error if "releaseRules" reference invalid commit type`,
			yaml: analyzerConfig("releaseRules: [{type: feat, release: invalid}]"),
		},
		{
			name: `load-release-rules.test.js: Throw error if a rule in "releaseRules" does not have a release type`,
			yaml: analyzerConfig("releaseRules: [{type: feat}]"),
		},
		{
			name: `load-release-rules.test.js: Throw error if "releaseRules" contains an undefined rule`,
			yaml: analyzerConfig("releaseRules: [null]"),
		},
		{
			name: "branches.test.js: Throw SemanticReleaseError for invalid configurations (EPRERELEASEBRANCH)",
			yaml: branchesConfig(`[master, {name: beta, prerelease: ""}]`),
		},
		{
			name: "branches.test.js: Throw SemanticReleaseError for invalid configurations (EPRERELEASEBRANCHES)",
			yaml: branchesConfig("[master, {name: alpha, prerelease: alpha}, {name: preview, prerelease: alpha}]"),
		},
		{
			name: "branches.test.js: Throw SemanticReleaseError for invalid configurations (ERELEASEBRANCHES)",
			yaml: branchesConfig("[{name: alpha, prerelease: alpha}]"),
		},
		// Upstream accepts up to 3 release branches and gives each one a
		// version range. The release command has no range logic, so it
		// rejects more than one release branch.
		{
			name: "normalize.test.js: Release branches - initial state",
			yaml: branchesConfig("[master, next, next-major]"),
		},
		{
			name: "branches.test.js: Throw a SemanticReleaseError if there is duplicate branches",
			yaml: branchesConfig("[master, master]"),
		},
		{
			name: "branches.test.js: Throw a SemanticReleaseError for each invalid branch name",
			yaml: branchesConfig(`["~master", "^master"]`),
		},
	} {
		tests[tt.name] = tt.yaml
	}
	// micromatch 4.0.8, which commit-analyzer uses for rule fields, gives
	// these globs a meaning that path.Match does not have:
	// {a,b} matches "a", !a matches "b", ** matches "a/b", @(a|b) matches
	// "a", +(a) matches "aa", a[ matches "a[", and a\b matches "a\b". An
	// empty pattern makes micromatch throw.
	for _, pattern := range []string{"{a,b}", "!a", "**", "@(a|b)", "+(a)", "a[", `a\b`, ""} {
		tests["micromatch glob "+strconv.Quote(pattern)] = analyzerConfig(
			"releaseRules: [{scope: '" + pattern + "', release: patch}]")
	}
	for name, cfg := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := Parse([]byte(cfg))
			assert.ErrorIs(t, err, ErrUnsupported)
		})
	}
}

func TestParseWarnsAboutLowercaseDryRun(t *testing.T) {
	cfg, err := Parse([]byte(`
preset: conventionalcommits
branches: [main]
plugins: [["@semantic-release/exec", {publishCmd: "DRYRUN=${options.dryrun} ./publish.sh"}]]`))
	assert.NilError(t, err)
	assert.Equal(t, len(cfg.Warnings), 1)
}

func TestParseKeepsOtherCommandsOfTheCheckerHook(t *testing.T) {
	_, err := Parse([]byte(`
preset: conventionalcommits
branches: [main]
plugins:
  - - "@semantic-release/exec"
    - generateNotesCmd: ./scripts/shell-wrapper.sh major-release-checker.js ${lastRelease.version} ${nextRelease.version}
      publishCmd: echo ${nextRelease.notes}`))
	assert.ErrorIs(t, err, ErrUnsupported)
}
