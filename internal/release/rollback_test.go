// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Tests for Rollback.

package release

import (
	"context"
	"os"
	"testing"

	"github.com/getoutreach/devbase/v2/internal/release/forge"
	"gotest.tools/v3/assert"
)

// rollback runs Rollback with the fixture's remote, fake forge and stubs.
func (p *publishFixture) rollback(opts *RollbackOptions) ([]string, error) {
	p.t.Helper()
	opts.Remote, opts.Forge, opts.Env, opts.Exec = p.remote, p.forge, os.Environ(), p.exec()
	return Rollback(context.Background(), p.repo(), opts)
}

// partialRelease publishes a release that fails at the upload, which
// leaves a draft release, the tag and the note.
func (p *publishFixture) partialRelease() *Plan {
	p.t.Helper()
	repo, plan := p.planAndBuild()
	p.forge.FailNext("UploadAsset", forge.ErrForge)
	assert.ErrorIs(p.t, p.publish(repo, plan), forge.ErrForge)
	return plan
}

func TestRollback(t *testing.T) {
	p := newPublishFixture(t, "")
	p.partialRelease()
	want := []string{
		"Fake release https://forge.test/releases/v1.0.0",
		"remote refs/tags/v1.0.0",
		"remote refs/notes/semantic-release-v1.0.0",
	}

	// A dry run deletes nothing.
	deleted, err := p.rollback(&RollbackOptions{Tag: "v1.0.0", DryRun: true})
	assert.NilError(t, err)
	assert.DeepEqual(t, deleted, want)
	assert.Assert(t, p.forge.Release("v1.0.0") != nil)
	assert.Assert(t, p.remoteRef("refs/tags/v1.0.0") != "")

	deleted, err = p.rollback(&RollbackOptions{Tag: "v1.0.0"})
	assert.NilError(t, err)
	assert.DeepEqual(t, deleted, want)
	assert.Assert(t, p.forge.Release("v1.0.0") == nil)
	assert.Equal(t, p.remoteRef("refs/tags/v1.0.0"), "")
	assert.Equal(t, p.remoteRef("refs/notes/semantic-release-v1.0.0"), "")
	assert.Equal(t, p.run("tag", "--list", "v1.0.0"), "")

	// The next run releases the same version again (scenario 13), and a
	// second rollback has nothing to do.
	repo, plan := p.planAndBuild()
	assert.Equal(t, plan.NextRelease.Version, "1.0.0")
	assert.NilError(t, p.publish(repo, plan))
	assert.Assert(t, !p.forge.Release("v1.0.0").Draft)
	deleted, err = p.rollback(&RollbackOptions{Tag: "v2.0.0"})
	assert.NilError(t, err)
	assert.Equal(t, len(deleted), 0)
}

func TestRollbackRefused(t *testing.T) {
	p := newPublishFixture(t, "")
	repo, plan := p.planAndBuild()
	assert.NilError(t, p.publish(repo, plan))

	for _, tag := range []string{unstableTag, "1.0.0", "vnext"} {
		_, err := p.rollback(&RollbackOptions{Tag: tag})
		assert.ErrorIs(t, err, ErrRollbackRefused, tag)
	}

	// A published release needs --force.
	_, err := p.rollback(&RollbackOptions{Tag: "v1.0.0"})
	assert.ErrorIs(t, err, ErrRollbackRefused)
	assert.ErrorContains(t, err, "is published")
	assert.Assert(t, p.remoteRef("refs/tags/v1.0.0") != "")

	_, err = p.rollback(&RollbackOptions{Tag: "v1.0.0", Force: true})
	assert.NilError(t, err)
	assert.Equal(t, p.remoteRef("refs/tags/v1.0.0"), "")
}

func TestRollbackReleaseCommit(t *testing.T) {
	p := newPublishFixture(t, `  - - "@semantic-release/exec"
    - prepareCmd: "echo ${nextRelease.version} > version.txt"
  - - "@semantic-release/git"
    - assets: [version.txt]
`)
	writeFile(t, p.git.Dir, "version.txt", "0.0.0\n")
	p.run("add", "version.txt")
	p.run("commit", "-q", "-m", "fix: add version file")
	p.run("push", "-q", "origin", "release")
	p.partialRelease()

	_, err := p.rollback(&RollbackOptions{Tag: "v1.0.0"})
	assert.ErrorIs(t, err, ErrRollbackRefused)
	assert.ErrorContains(t, err, "release commit "+p.remoteRef("refs/tags/v1.0.0"))
}
