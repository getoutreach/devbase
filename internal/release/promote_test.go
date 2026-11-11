// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Tests for Promote.

package release

import (
	"context"
	"testing"

	"gotest.tools/v3/assert"
)

// promote runs Promote with the fixture's remote.
func (p *publishFixture) promote(opts *PromoteOptions) (*Tag, error) {
	p.t.Helper()
	opts.Remote = p.remote
	return Promote(context.Background(), p.repo(), opts)
}

// tagAndPush tags HEAD and pushes the tag.
func (p *publishFixture) tagAndPush(tag string) string {
	p.t.Helper()
	p.run("tag", tag)
	p.run("push", "-q", "origin", tag)
	return p.run("rev-parse", "HEAD")
}

func TestPromote(t *testing.T) {
	p := newPublishFixture(t, "")
	p.tagAndPush("v1.0.0")
	p.run("checkout", "-q", "main")
	p.commit("feat: one")
	rc1 := p.tagAndPush("v1.1.0-rc.1")
	p.commit("fix: two")
	rc2 := p.tagAndPush("v1.1.0-rc.2")
	p.commit("fix: not a candidate yet")
	p.run("push", "-q", "origin", "main")
	before := p.remoteRef("refs/heads/release")

	// A dry run pushes nothing.
	tag, err := p.promote(&PromoteOptions{DryRun: true})
	assert.NilError(t, err)
	assert.Equal(t, tag.Name, "v1.1.0-rc.2")
	assert.Equal(t, p.remoteRef("refs/heads/release"), before)

	// --tag picks an older candidate; the default is the latest one.
	tag, err = p.promote(&PromoteOptions{Tag: "v1.1.0-rc.1"})
	assert.NilError(t, err)
	assert.Equal(t, tag.Name, "v1.1.0-rc.1")
	assert.Equal(t, p.remoteRef("refs/heads/release"), rc1)
	_, err = p.promote(&PromoteOptions{})
	assert.NilError(t, err)
	assert.Equal(t, p.remoteRef("refs/heads/release"), rc2)
}

func TestPromoteNoCandidate(t *testing.T) {
	p := newPublishFixture(t, "")
	p.run("checkout", "-q", "main")
	p.commit("feat: one")
	p.tagAndPush("v1.0.0-rc.1")
	p.run("checkout", "-q", "release")
	p.run("merge", "-q", "main")
	p.tagAndPush("v1.0.0")
	p.run("push", "-q", "origin", "release", "main")

	_, err := p.promote(&PromoteOptions{})
	assert.ErrorIs(t, err, ErrNoCandidate)
	_, err = p.promote(&PromoteOptions{Tag: "v1.0.0"})
	assert.ErrorIs(t, err, ErrNotCandidate)
}
