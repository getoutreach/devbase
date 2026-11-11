// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Tests for the .releaserc.yaml loader.

package release

import (
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

func TestParseConfig(t *testing.T) {
	cfg, err := ParseConfig([]byte(prereleaseConfig))
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

func TestParseConfigFailsClosed(t *testing.T) {
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
	for name, cfg := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := ParseConfig([]byte(cfg))
			assert.ErrorIs(t, err, ErrUnsupportedConfig)
		})
	}
}

func TestParseConfigWarnsAboutLowercaseDryRun(t *testing.T) {
	cfg, err := ParseConfig([]byte(`
preset: conventionalcommits
branches: [main]
plugins: [["@semantic-release/exec", {publishCmd: "DRYRUN=${options.dryrun} ./publish.sh"}]]`))
	assert.NilError(t, err)
	assert.Equal(t, len(cfg.Warnings), 1)
}

func TestParseConfigKeepsOtherCommandsOfTheCheckerHook(t *testing.T) {
	_, err := ParseConfig([]byte(`
preset: conventionalcommits
branches: [main]
plugins:
  - - "@semantic-release/exec"
    - generateNotesCmd: ./scripts/shell-wrapper.sh major-release-checker.js ${lastRelease.version} ${nextRelease.version}
      publishCmd: echo ${nextRelease.notes}`))
	assert.ErrorIs(t, err, ErrUnsupportedConfig)
}
