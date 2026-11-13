// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Tests for Preview. The decidePreview and base tests port
// the cases of shell/lib/release_test.bats.

package release

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/getoutreach/devbase/v2/internal/release/forge/github"
	"gotest.tools/v3/assert"
)

// action runs decidePreview for base and head in the fixture.
func (f *fixture) action(base, head string) (previewAction, error) {
	f.t.Helper()
	a, _, err := decidePreview(context.Background(), f.git, base, head)
	return a, err
}

// writeCommit writes a file and commits it with msg.
func (f *fixture) writeCommit(name, content, msg string) {
	f.t.Helper()
	writeFile(f.t, f.git.Dir, name, content)
	f.run("add", name)
	f.run("commit", "-q", "-m", msg)
}

// newActionRepo returns a repository with a root commit on main.
func newActionRepo(t *testing.T) *fixture {
	t.Helper()
	f := newRepo(t, "main")
	f.commit("root")
	return f
}

// ancestorHead leaves the repository on a branch "tmp" that is an
// ancestor of main.
func ancestorHead(f *fixture) {
	f.run("tag", "rc")
	f.commit("newer on main")
	f.run("checkout", "-q", "-b", "tmp", "rc")
}

func TestPreviewAction(t *testing.T) {
	tests := []struct {
		name  string
		setup func(f *fixture) (head string)
		want  previewAction
	}{{
		name: "head has a real delta over base",
		setup: func(f *fixture) string {
			f.run("checkout", "-q", "-b", "feature")
			f.writeCommit("work.txt", "feature work\n", "feature work")
			return "feature"
		},
		want: previewSquash,
	}, {
		name: "head was squash-merged into base",
		setup: func(f *fixture) string {
			f.run("checkout", "-q", "-b", "feature")
			f.writeCommit("work.txt", "feature work\n", "feature work")
			f.run("checkout", "-q", "main")
			f.writeCommit("work.txt", "feature work\n", "feat!: feature work (#1)")
			return "feature"
		},
		want: previewMerged,
	}, {
		name: "head was rebase-merged into base",
		setup: func(f *fixture) string {
			f.run("checkout", "-q", "-b", "feature")
			f.writeCommit("work.txt", "feature work\n", "feat!: feature work")
			f.run("checkout", "-q", "main")
			f.commit("newer on main")
			f.run("cherry-pick", "feature")
			return "feature"
		},
		want: previewMerged,
	}, {
		name:  "head is an ancestor of base",
		setup: func(f *fixture) string { ancestorHead(f); return "tmp" },
		want:  previewNone,
	}, {
		name: "head change is already in base",
		setup: func(f *fixture) string {
			f.run("checkout", "-q", "-b", "feature")
			f.writeCommit("shared.txt", "shared\n", "add shared on feature")
			f.run("checkout", "-q", "main")
			f.writeCommit("shared.txt", "shared\n", "add identical shared on main")
			return "feature"
		},
		// The head changes the tree since the fork point, so it is
		// previewed as merged, which runs the check.
		want: previewMerged,
	}, {
		name: "head only adds empty commits",
		setup: func(f *fixture) string {
			f.run("checkout", "-q", "-b", "feature")
			f.commit("chore: nothing")
			f.run("checkout", "-q", "main")
			f.commit("newer on main")
			return "feature"
		},
		want: previewNone,
	}, {
		name: "head was merge-committed into base (known limit)",
		setup: func(f *fixture) string {
			f.run("checkout", "-q", "-b", "feature")
			f.writeCommit("work.txt", "feature work\n", "feature work")
			f.run("checkout", "-q", "main")
			f.run("merge", "-q", "--no-ff", "feature", "-m", "Merge feature")
			return "feature"
		},
		want: previewNone,
	}}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := newActionRepo(t)
			head := tc.setup(f)
			got, err := f.action("main", head)
			assert.NilError(t, err)
			assert.Equal(t, got, tc.want)
		})
	}
}

func TestPreviewActionErrors(t *testing.T) {
	// A conflict names the conflicted file.
	f := newActionRepo(t)
	f.writeCommit("f.txt", "line1\n", "base line")
	f.run("checkout", "-q", "-b", "feature")
	f.writeCommit("f.txt", "feature-version\n", "feature change")
	f.run("checkout", "-q", "main")
	f.writeCommit("f.txt", "main-version\n", "main change")
	_, err := f.action("main", "feature")
	assert.ErrorIs(t, err, ErrPreviewConflict)
	assert.ErrorContains(t, err, "f.txt")

	// A ref that does not exist is not a conflict.
	_, err = f.action("main", "does-not-exist")
	assert.ErrorIs(t, err, ErrPreviewBase)
}

func TestHeadHasNetChanges(t *testing.T) {
	ctx := context.Background()

	// git diff fails: removing the tree of head breaks the diff, but not
	// the merge base, which reads only commits.
	f := newActionRepo(t)
	f.run("checkout", "-q", "-b", "feature")
	f.writeCommit("work.txt", "feature work\n", "feature work")
	f.run("checkout", "-q", "main")
	tree := f.run("rev-parse", "feature^{tree}")
	assert.NilError(t, os.Remove(filepath.Join(f.git.Dir, ".git", "objects", tree[:2], tree[2:])))
	assert.Assert(t, headHasNetChanges(ctx, f.git, "main", "feature"))

	// The merge base cannot be found.
	assert.Assert(t, headHasNetChanges(ctx, f.git, "main", "does-not-exist"))

	// head is an ancestor of base.
	f = newActionRepo(t)
	ancestorHead(f)
	assert.Assert(t, !headHasNetChanges(ctx, f.git, "main", "tmp"))
}

// newPreviewFixture returns a publish fixture on main with a remote and
// a feature branch that adds a feature in two commits.
func newPreviewFixture(t *testing.T) *publishFixture {
	t.Helper()
	p := newPublishFixture(t, "")
	p.run("checkout", "-q", "main")
	p.run("checkout", "-q", "-b", "feature")
	p.writeCommit("a.txt", "a\n", "feat: first")
	p.writeCommit("b.txt", "b\n", "fix: second")
	return p
}

// preview runs Preview on head with base.
func (p *publishFixture) preview(base, head string) (*Plan, error) {
	p.t.Helper()
	return Preview(context.Background(), p.repo(), &PreviewOptions{
		Base: base, Head: head, Remote: p.remote, NewLinks: github.NewLinks,
	})
}

func TestPreview(t *testing.T) {
	p := newPreviewFixture(t)
	head := p.run("rev-parse", "HEAD")
	plan, err := p.preview("", "")
	assert.NilError(t, err)

	// The plan is for the squash commit on main, by Devbase CI, with the
	// messages in order.
	assert.Equal(t, plan.Branch.Name, "main")
	assert.Assert(t, plan.Release, plan.Reason)
	assert.Equal(t, plan.NextRelease.Version, "1.0.0-rc.1")
	assert.Equal(t, p.run("log", "-1", "--format=%an%n%B", plan.Commit), "Devbase CI\nfeat: first\n\nfix: second")
	assert.Equal(t, p.run("show", plan.Commit+":b.txt"), "b")

	// The checkout did not change, and the worktree is gone.
	assert.Equal(t, p.run("rev-parse", "HEAD"), head)
	assert.Equal(t, p.run("status", "--porcelain"), "")
	assert.Equal(t, strings.Count(p.run("worktree", "list"), "\n"), 0)
}

func TestPreviewConfigOfHead(t *testing.T) {
	// The pull request adds a release rule for docs and a docs commit:
	// the preview uses the config of the squash commit.
	p := newPublishFixture(t, "")
	p.run("checkout", "-q", "main")
	p.tagAndPush("v1.0.0")
	p.run("checkout", "-q", "-b", "feature")
	p.writeCommit("README.md", "docs\n", "docs: readme")
	plan, err := p.preview("", "")
	assert.NilError(t, err)
	assert.Assert(t, !plan.Release)

	b, err := os.ReadFile(filepath.Join(p.git.Dir, ".releaserc.yaml"))
	assert.NilError(t, err)
	rules := "  - - \"@semantic-release/commit-analyzer\"\n    - releaseRules:\n        - type: docs\n          release: patch\n"
	cfg := strings.Replace(string(b), "  - \"@semantic-release/commit-analyzer\"\n", rules, 1)
	assert.Assert(t, cfg != string(b))
	p.writeCommit(".releaserc.yaml", cfg, "chore: release docs")
	plan, err = p.preview("", "")
	assert.NilError(t, err)
	assert.Assert(t, plan.Release, plan.Reason)
}

func TestPreviewMergedAndNone(t *testing.T) {
	p := newPreviewFixture(t)
	p.run("checkout", "-q", "main")
	p.run("merge", "-q", "--squash", "feature")
	p.run("commit", "-q", "-m", "feat: feature (#1)")
	p.run("push", "-q", "origin", "main")
	plan, err := p.preview("", "feature")
	assert.NilError(t, err)
	assert.Equal(t, plan.Commit, p.run("rev-parse", "main"))

	plan, err = p.preview("", "main~1")
	assert.NilError(t, err)
	assert.Assert(t, plan == nil)
}

func TestPreviewBase(t *testing.T) {
	p := newPreviewFixture(t)
	p.run("checkout", "-q", "main")
	writeFile(t, p.git.Dir, "service.yaml", unstableServiceYAML)
	p.run("checkout", "-q", "-b", "promote", "main")
	repo := p.repo()
	ctx := context.Background()

	// A branch at an older commit of main is a stable promotion.
	p.run("checkout", "-q", "main")
	p.commit("feat: newer on main")
	p.run("push", "-q", "origin", "main")
	p.run("checkout", "-q", "promote")
	base, err := repo.previewBase(ctx, p.remote, "promote")
	assert.NilError(t, err)
	assert.Equal(t, base, "release")

	// A branch ahead of main, such as an rc branch or a feature branch,
	// uses the default branch.
	p.commit("chore: Release RC")
	base, err = repo.previewBase(ctx, p.remote, "promote")
	assert.NilError(t, err)
	assert.Equal(t, base, "main")

	// Without prereleases, an ancestor uses the default branch.
	writeFile(t, p.git.Dir, "service.yaml", "name: widgets\n")
	base, err = repo.previewBase(ctx, p.remote, "main~1")
	assert.NilError(t, err)
	assert.Equal(t, base, "main")
}

func TestPreviewBaseErrors(t *testing.T) {
	p := newPreviewFixture(t)
	_, err := p.preview("missing", "")
	assert.ErrorIs(t, err, ErrPreviewBase)
	_, err = p.preview("-bad", "")
	assert.ErrorIs(t, err, ErrPreviewBase)

	// A branch with no common history has no merge base.
	p.run("checkout", "-q", "--orphan", "other")
	p.commit("unrelated root")
	_, err = p.preview("main", "")
	assert.ErrorIs(t, err, ErrPreviewBase)
}

// gemPreview runs Preview of the checked-out branch against main, with
// the stubs.
func (g *gemRepoFixture) gemPreview() (*Plan, string, error) {
	g.t.Helper()
	var log strings.Builder
	plan, err := Preview(context.Background(), g.repo(), &PreviewOptions{
		Base: "main", Remote: g.remote, NewLinks: github.NewLinks, Exec: g.exec(), Log: &log,
	})
	return plan, log.String(), err
}

// TestPreviewBuildsGem checks that the preview builds the gem in the
// temporary worktree with the planned root version, or the last root
// version when the root does not release, and that the checkout does
// not change.
func TestPreviewBuildsGem(t *testing.T) {
	g := newGemRepoFixture(t)
	g.run("checkout", "-q", "-b", "feature")
	head := g.run("rev-parse", "HEAD")

	tests := []struct {
		name, file, msg, want string
		release, root         bool
	}{
		// The root releases 1.1.0.
		{name: "root release", file: "a.txt", msg: "feat: widget", want: "1.1.0", release: true, root: true},
		// Only the unit releases, as the root excludes node/**.
		{name: "unit release", file: "node/widgets-client/index.js", msg: "fix: client", want: "1.0.0", release: true},
		// Nothing releases, but the gem is built as on every branch.
		{name: "no release", file: "README.md", msg: "docs: readme", want: "1.0.0"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g.run("reset", "-q", "--hard", head)
			g.writeCommit(tc.file, tc.msg+"\n", tc.msg)
			before := g.run("rev-parse", "HEAD")
			g.cmds = newCmdStubs(t)
			expectGemBuild(g.cmds, rakeBuildWrites("widgets", "lib/version.rb"))
			plan, log, err := g.gemPreview()
			assert.NilError(t, err)
			assert.Equal(t, plan.Release, tc.release)
			assert.Equal(t, plan.RootRelease(), tc.root)
			assert.Equal(t, buildVersion(plan), tc.want)

			for _, c := range g.cmds.calls() {
				dir := c.Cmd.Dir
				assert.Assert(t, strings.HasSuffix(dir, filepath.Join("worktree", "ruby-grpc", "widgets")), dir)
				assert.Assert(t, !strings.HasPrefix(dir, g.git.Dir), dir)
			}
			assert.Assert(t, strings.Contains(log, "Build check: widgets "+tc.want+"\n"), log)
			assert.Assert(t, strings.Contains(log, "widgets "+tc.want+" built to pkg/widgets-"+tc.want+".gem.\n"), log)

			// The checkout did not change, and the worktree is gone.
			assert.Equal(t, g.run("rev-parse", "HEAD"), before)
			assert.Equal(t, g.run("status", "--porcelain", "--ignored"), "")
			assert.Equal(t, strings.Count(g.run("worktree", "list"), "\n"), 0)
		})
	}
	assert.Equal(t, buildVersion(&Plan{}), "0.0.0")
}

func TestPreviewGemBuildFails(t *testing.T) {
	g := newGemRepoFixture(t)
	g.run("checkout", "-q", "-b", "feature")
	g.writeCommit("a.txt", "a\n", "feat: widget")
	expectGemBuild(g.cmds, cmdFails("rake aborted!"))
	_, _, err := g.gemPreview()
	assert.ErrorContains(t, err, "build check: bundle exec rake build: bundle exec rake build: exit status 1: rake aborted!")
	assert.Equal(t, g.run("status", "--porcelain", "--ignored"), "")

	// A pull request that adds no change builds nothing: the stubs have
	// no more commands.
	g.run("checkout", "-q", "main")
	plan, _, err := g.gemPreview()
	assert.NilError(t, err)
	assert.Assert(t, plan == nil)
}
