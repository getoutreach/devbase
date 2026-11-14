// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Tests for the unstable release.

package release

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gotest.tools/v3/assert"
)

// unstableServiceYAML enables prereleases on main.
const unstableServiceYAML = `name: widgets
arguments:
  releaseOptions:
    enablePrereleases: true
    prereleasesBranch: main
`

// newUnstableFixture returns a publish fixture on main with service.yaml,
// an include script that records DRYRUN, and a fake go command.
func newUnstableFixture(t *testing.T) *publishFixture {
	t.Helper()
	grFakeGo(t)
	p := newPublishFixture(t, "")
	p.run("checkout", "-q", "main")
	writeFile(t, p.git.Dir, "service.yaml", unstableServiceYAML)
	writeFile(t, p.git.Dir, unstableInclude, "#!/bin/sh\necho \"DRYRUN=$DRYRUN\" > include.log\n")
	assert.NilError(t, os.Chmod(filepath.Join(p.git.Dir, unstableInclude), 0o755))
	writeFile(t, p.git.Dir, "cmd/hello/main.go", "package main\n")
	p.run("add", ".")
	p.run("commit", "-q", "-m", "chore: service")
	p.run("push", "-q", "origin", "main")
	return p
}

// unstable runs Unstable on branch.
func (p *publishFixture) unstable(branch string) error {
	p.t.Helper()
	return Unstable(context.Background(), p.repo(), &UnstableOptions{
		Branch: branch, Dist: "dist", Remote: p.remote, Forge: p.forge, Env: os.Environ(),
	})
}

func TestUnstable(t *testing.T) {
	p := newUnstableFixture(t)
	assert.NilError(t, p.unstable("main"))
	head := p.run("rev-parse", "HEAD")

	rel := p.forge.Release(unstableTag)
	assert.Assert(t, rel != nil && !rel.Draft)
	assert.DeepEqual(t, assetNames(rel), []string{"widgets_0.0.0-unstable+" + head + "_linux_amd64.tar.gz", "checksums.txt"})
	assert.Equal(t, p.remoteRef("refs/tags/unstable"), head)
	b, err := os.ReadFile(filepath.Join(p.git.Dir, "include.log"))
	assert.NilError(t, err)
	assert.Equal(t, string(b), "DRYRUN=false\n")

	// The next run replaces the release and moves the tag.
	p.commit("fix: next")
	p.run("push", "-q", "origin", "main")
	assert.NilError(t, p.unstable("main"))
	next := p.run("rev-parse", "HEAD")
	assert.Equal(t, p.remoteRef("refs/tags/unstable"), next)
	assert.Assert(t, strings.Contains(assetNames(p.forge.Release(unstableTag))[0], next))
	assert.Assert(t, strings.Contains(strings.Join(p.forge.Calls, " "), "DeleteRelease"))
}

// TestUnstableNativeUnstableOnly checks that a release section with a
// build of only the unstable release builds the unstable release.
func TestUnstableNativeUnstableOnly(t *testing.T) {
	p := newUnstableFixture(t)
	assert.NilError(t, os.Remove(filepath.Join(p.git.Dir, ".releaserc.yaml")))
	assert.NilError(t, os.Remove(filepath.Join(p.git.Dir, ".goreleaser.yml")))
	writeFile(t, p.git.Dir, "scripts/devbase.yaml", `release:
  branches:
    - name: main
      prerelease: rc
    - name: release
  build:
    unstableOnly: true
    before: []
    builds:
      - binary: hello
        ldflags: [-s -w]
    targets: [linux/amd64]
`)
	repo := p.repo()
	assert.Assert(t, repo.Config.Artifacts == nil)
	assert.NilError(t, p.unstable("main"))
	head := p.run("rev-parse", "HEAD")
	assert.DeepEqual(t, assetNames(p.forge.Release(unstableTag)),
		[]string{"widgets_0.0.0-unstable+" + head + "_linux_amd64.tar.gz", "checksums.txt"})
}

// TestUnstableNoBuild checks that the unstable release without a build
// names the configuration that has no build, and runs only the include
// script.
func TestUnstableNoBuild(t *testing.T) {
	p := newUnstableFixture(t)
	assert.NilError(t, os.Remove(filepath.Join(p.git.Dir, ".goreleaser.yml")))
	writeFile(t, p.git.Dir, ".releaserc.yaml", fixtureConfig)
	var legacy strings.Builder
	assert.NilError(t, Unstable(context.Background(), p.repo(), &UnstableOptions{
		Branch: "main", Remote: p.remote, Forge: p.forge, Env: os.Environ(), Log: &legacy,
	}))
	assert.Assert(t, strings.Contains(legacy.String(), "No .goreleaser.yml, skipping creating unstable release"),
		legacy.String())

	assert.NilError(t, os.Remove(filepath.Join(p.git.Dir, ".releaserc.yaml")))
	writeFile(t, p.git.Dir, "scripts/devbase.yaml", "release:\n  branches:\n    - name: main\n")
	var section strings.Builder
	assert.NilError(t, Unstable(context.Background(), p.repo(), &UnstableOptions{
		Branch: "main", Remote: p.remote, Forge: p.forge, Env: os.Environ(), Log: &section,
	}))
	assert.Assert(t, strings.Contains(section.String(),
		"The release section of scripts/devbase.yaml has no build key, skipping creating unstable release"),
		section.String())
	assert.Equal(t, len(p.forge.Calls), 0)
	_, err := os.Stat(filepath.Join(p.git.Dir, "include.log"))
	assert.NilError(t, err)
}

func TestUnstableSkips(t *testing.T) {
	p := newUnstableFixture(t)

	// Another branch, and a dry run, do nothing.
	assert.NilError(t, p.unstable("feature"))
	assert.NilError(t, Unstable(context.Background(), p.repo(), &UnstableOptions{Branch: "main", DryRun: true}))
	assert.Equal(t, len(p.forge.Calls), 0)

	// Without enablePrereleases, nothing runs.
	writeFile(t, p.git.Dir, "service.yaml", "name: widgets\n")
	assert.NilError(t, p.unstable("main"))
	assert.Equal(t, len(p.forge.Calls), 0)
	_, err := os.Stat(filepath.Join(p.git.Dir, "include.log"))
	assert.Assert(t, os.IsNotExist(err))
}
