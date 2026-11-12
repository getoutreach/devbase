// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Tests for the .releaserc.yaml loader.

package releaserc

import (
	"strconv"
	"strings"
	"testing"

	"github.com/getoutreach/devbase/v2/internal/release/spec"
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
	cfg, warnings, err := Parse([]byte(prereleaseConfig))
	assert.NilError(t, err)

	assert.DeepEqual(t, cfg, &spec.Config{
		Branches: []spec.Branch{
			{Name: "main", Prerelease: "rc", Channel: "main"},
			{Name: "release", Channel: ""},
		},
		Rules: []spec.Rule{
			{Breaking: true, Release: "minor"},
			{Type: "revert", Release: "patch"},
		},
		MajorCheck: true, // major-release-checker.js becomes the built-in check.
		Build:      true, // The make release hook becomes the build command.
		// The plugins keep their order. The checker and build hooks have
		// no other command, so their exec plugins are left out.
		Plugins: []spec.Plugin{
			{Notes: &spec.NotesPlugin{}},
			{Forge: &spec.ForgePlugin{Provider: spec.ForgeGitHub, Assets: []string{"dist/*.tar.gz", "dist/checksums.txt"}}},
			{Orb: &spec.OrbPlugin{Name: "getoutreach/shared", Path: "orb.yml"}},
		},
	})
	assert.Equal(t, len(warnings), 0)
	assert.Equal(t, cfg.Forge().Provider, spec.ForgeGitHub)

	// The hook that runs the release:build mise task is the build too.
	task := strings.Replace(prereleaseConfig, `"make release APP_VERSION=${nextRelease.version}"`,
		`"APP_VERSION=${nextRelease.version} mise run --quiet release:build 1>&2"`, 1)
	assert.Assert(t, task != prereleaseConfig)
	got, _, err := Parse([]byte(task))
	assert.NilError(t, err)
	assert.DeepEqual(t, got, cfg)

	// Another form of the build hook fails closed.
	other := strings.Replace(prereleaseConfig, `"make release APP_VERSION=${nextRelease.version}"`,
		`"make release APP_VERSION=${nextRelease.version} 1>&2"`, 1)
	_, _, err = Parse([]byte(other))
	assert.ErrorIs(t, err, spec.ErrUnsupported)
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
			_, _, err := Parse([]byte(cfg))
			assert.ErrorIs(t, err, spec.ErrUnsupported)
		})
	}
}

func TestParseWarnsAboutLowercaseDryRun(t *testing.T) {
	_, warnings, err := Parse([]byte(`
preset: conventionalcommits
branches: [main]
plugins: [["@semantic-release/exec", {publishCmd: "DRYRUN=${options.dryrun} ./publish.sh"}]]`))
	assert.NilError(t, err)
	assert.Equal(t, len(warnings), 1)
}

func TestParseKeepsOtherCommandsOfTheCheckerHook(t *testing.T) {
	_, _, err := Parse([]byte(`
preset: conventionalcommits
branches: [main]
plugins:
  - - "@semantic-release/exec"
    - generateNotesCmd: ./scripts/shell-wrapper.sh major-release-checker.js ${lastRelease.version} ${nextRelease.version}
      publishCmd: echo ${nextRelease.notes}`))
	assert.ErrorIs(t, err, spec.ErrUnsupported)
}

// TestParseDropsClientVersionFiles checks that the npm client version file
// leaves the release commit with a warning (requirement 12), and that the
// other assets stay.
func TestParseDropsClientVersionFiles(t *testing.T) {
	cfg, warnings, err := Parse([]byte(`preset: conventionalcommits
branches: [main]
plugins:
  - - "@semantic-release/npm"
    - pkgRoot: api/clients/node
  - - "@semantic-release/git"
    - assets:
        - api/clients/node/package.json
        - go.mod
`))
	assert.NilError(t, err)
	assert.DeepEqual(t, cfg.Plugins[1], spec.Plugin{Commit: &spec.CommitPlugin{Files: []string{"go.mod"}}})
	assert.DeepEqual(t, warnings, []string{
		`the release does not commit the client version file "api/clients/node/package.json"; ` +
			"remove it from the release commit files",
	})
}
