// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Tests for LoadRepo: which configuration it reads, the
// digest, and the conversion of the old files.

package release

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/getoutreach/devbase/v2/internal/release/goreleaser"
	"github.com/getoutreach/devbase/v2/internal/release/native"
	"github.com/getoutreach/devbase/v2/internal/release/releaserc"
	"gotest.tools/v3/assert"
)

// fixtureSection is the release section that is equivalent to
// fixtureConfig, after a graphql section.
const fixtureSection = `graphql:
  lint: {}
release:
  branches:
    - name: main
      prerelease: rc
    - name: release
  allowMajorVersions: true
  rules:
    - breaking: true
      release: minor
    - type: revert
      release: patch
    - type: perf
      release: patch
  publishers:
    - notes
`

// fixtureGoReleaser is a .goreleaser.yml with one build.
const fixtureGoReleaser = `builds:
  - main: ./cmd/app
    id: app
    binary: app
    goos: [linux]
    goarch: [amd64]
    ldflags: [-s -w]
checksum:
  name_template: checksums.txt
`

// writeFiles writes files, by path relative to dir, and fails the test on
// an error.
func writeFiles(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for name, contents := range files {
		path := filepath.Join(dir, filepath.FromSlash(name))
		assert.NilError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		assert.NilError(t, os.WriteFile(path, []byte(contents), 0o600))
	}
}

// digest returns the digest of config files, given as name and contents
// pairs, as LoadRepo computes it.
func digest(files ...string) string {
	h := sha256.New()
	for _, f := range files {
		h.Write([]byte(f))
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil))
}

func TestLoadRepoReadsTheReleaseSection(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{native.File: fixtureSection})
	repo, err := LoadRepo(dir)
	assert.NilError(t, err)

	// The section loads as the same model as .releaserc.yaml.
	want, _, err := releaserc.Parse([]byte(fixtureConfig))
	assert.NilError(t, err)
	assert.DeepEqual(t, repo.Config, want)

	// The digest covers the whole file that the section came from.
	assert.Equal(t, repo.Digest, digest(native.File, fixtureSection))
}

func TestLoadRepoWithoutTheReleaseSection(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{releaserc.File: fixtureConfig})
	before, err := LoadRepo(dir)
	assert.NilError(t, err)
	assert.Equal(t, before.Digest, digest(releaserc.File, fixtureConfig))

	// A scripts/devbase.yaml without a release section changes nothing.
	writeFiles(t, dir, map[string]string{native.File: "graphql:\n  lint: {}\n"})
	after, err := LoadRepo(dir)
	assert.NilError(t, err)
	assert.DeepEqual(t, after, before)
}

// TestLoadRepoRejectsTwoConfigurations checks that the old files are an
// error next to a release section, and that the message names them.
func TestLoadRepoRejectsTwoConfigurations(t *testing.T) {
	for _, old := range [][]string{
		{releaserc.File},
		{goreleaser.File},
		{releaserc.File, goreleaser.File},
	} {
		dir := t.TempDir()
		files := map[string]string{native.File: fixtureSection}
		for _, name := range old {
			files[name] = ""
		}
		writeFiles(t, dir, files)
		_, err := LoadRepo(dir)
		assert.ErrorIs(t, err, ErrConfigConflict)
		assert.ErrorContains(t, err, "delete "+strings.Join(old, " and "))
	}
}

func TestLoadRepoReportsSectionErrors(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{native.File: fixtureSection + "  units: []\n"})
	_, err := LoadRepo(dir)
	assert.ErrorContains(t, err, "scripts/devbase.yaml: release: ")
	assert.ErrorContains(t, err, "field units not found")
}

// TestReadPlanChecksTheReleaseSection checks that a change to the section
// makes a plan stale, as a change to .releaserc.yaml does.
func TestReadPlanChecksTheReleaseSection(t *testing.T) {
	f := newFixture(t)
	f.run("rm", "-q", releaserc.File)
	writeFiles(t, f.git.Dir, map[string]string{native.File: fixtureSection})
	f.run("add", ".")
	f.run("commit", "-q", "-m", "chore: move the release configuration")
	f.run("branch", "release")
	f.run("checkout", "-q", "release")
	plan, err := f.plan()
	assert.NilError(t, err)
	assert.Equal(t, plan.NextRelease.Version, "1.0.0")
	file := filepath.Join(t.TempDir(), "plan.json")
	assert.NilError(t, WritePlan(file, plan))

	writeFiles(t, f.git.Dir, map[string]string{native.File: fixtureSection + "    - github\n"})
	repo, err := LoadRepo(f.git.Dir)
	assert.NilError(t, err)
	_, err = ReadPlan(file, repo)
	assert.ErrorIs(t, err, ErrPlanConfig)
}

// TestConvertConfig converts the old files, replaces them with the
// section, and checks that LoadRepo reads the same model.
func TestConvertConfig(t *testing.T) {
	cfg := strings.Replace(fixtureConfig, `  - "@semantic-release/release-notes-generator"`,
		`  - - "@semantic-release/exec"
    - generateNotesCmd: "make release APP_VERSION=${nextRelease.version}"
  - "@semantic-release/release-notes-generator"
  - - "@semantic-release/github"
    - assets: [dist/*.tar.gz, dist/checksums.txt]`, 1)
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{releaserc.File: cfg, goreleaser.File: fixtureGoReleaser})
	old, err := LoadRepo(dir)
	assert.NilError(t, err)
	assert.Assert(t, old.Config.Build)

	section, notes, err := ConvertConfig(dir)
	assert.NilError(t, err)
	assert.Equal(t, len(notes), 0)
	assert.NilError(t, os.Remove(filepath.Join(dir, releaserc.File)))
	assert.NilError(t, os.Remove(filepath.Join(dir, goreleaser.File)))
	writeFiles(t, dir, map[string]string{native.File: string(section)})
	repo, err := LoadRepo(dir)
	assert.NilError(t, err)
	assert.DeepEqual(t, repo.Config, old.Config)
}

// TestConvertConfigNotesUnstableOnlyBuild checks the note for a
// .goreleaser.yml that only the unstable release reads.
func TestConvertConfigNotesUnstableOnlyBuild(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{releaserc.File: fixtureConfig, goreleaser.File: fixtureGoReleaser})
	section, notes, err := ConvertConfig(dir)
	assert.NilError(t, err)
	assert.Assert(t, !strings.Contains(string(section), "build:"))
	assert.Equal(t, len(notes), 1)
	assert.Assert(t, strings.Contains(notes[0], "unstable release builds nothing"), notes[0])
}
