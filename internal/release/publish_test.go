// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Tests for Publish with a bare remote and a fake forge.

package release

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/getoutreach/devbase/v2/internal/release/forge"
	"github.com/getoutreach/devbase/v2/internal/release/forge/forgetest"
	"github.com/getoutreach/devbase/v2/internal/release/native"
	"gotest.tools/v3/assert"
)

// publishConfig is a stencil-style config with the build hook, GitHub
// release assets, and exec hooks that record their rendered fields.
const publishConfig = `preset: conventionalcommits
branches:
  - name: main
    prerelease: rc
  - name: release
plugins:
  - "@semantic-release/commit-analyzer"
  - - "@semantic-release/exec"
    - generateNotesCmd: "make release APP_VERSION=${nextRelease.version}"
  - "@semantic-release/release-notes-generator"
  - - "@semantic-release/exec"
    - prepareCmd: "echo prepare ${nextRelease.version} ${branch.name} >> hooks.log"
      publishCmd: "echo '{\"name\": \"registry\", \"url\": \"https://registry.test/${nextRelease.version}\"}'"
      successCmd: "echo success ${nextRelease.gitTag} ${nextRelease.channel} >> hooks.log"
  - - "@semantic-release/github"
    - assets:
        - "dist/*.tar.gz"
        - "dist/checksums.txt"
`

// publishGoReleaser builds one linux/amd64 binary.
const publishGoReleaser = `builds:
  - id: hello
    binary: hello
    goos: [linux]
    goarch: [amd64]
    ldflags: ["-s -w"]
checksum:
  name_template: checksums.txt
`

// publishFixture is a repository with a remote, a release branch with one
// fix after the first commit, and a fake forge.
type publishFixture struct {
	*fixture

	forge *forgetest.Fake
}

// newPublishFixture makes a publishFixture on the release branch. extra
// is added to the config.
func newPublishFixture(t *testing.T, extra string) *publishFixture {
	t.Helper()
	f := newRepo(t, "main")
	f.run("remote", "add", "origin", "https://github.com/acme/widgets.git")
	writeFile(t, f.git.Dir, ".releaserc.yaml", publishConfig+extra)
	writeFile(t, f.git.Dir, ".goreleaser.yml", publishGoReleaser)
	writeFile(t, f.git.Dir, ".gitignore", "dist/\nhooks.log\n")
	f.run("add", ".")
	f.run("commit", "-q", "-m", "feat: init")
	f.run("branch", "release")
	f.run("checkout", "-q", "release")
	f.commit("fix: a bug")
	f.withRemote()
	return &publishFixture{
		fixture: f,
		forge:   forgetest.New(),
	}
}

// planAndBuild plans a release and writes a fake archive for each planned
// asset in dist/<os>_<arch>/, as the build jobs do.
func (p *publishFixture) planAndBuild() (*Repo, *Plan) {
	p.t.Helper()
	plan, err := p.plan()
	assert.NilError(p.t, err)
	assert.Assert(p.t, plan.Release, plan.Reason)
	repo := p.repo()
	for _, asset := range plan.Assets {
		if strings.HasSuffix(asset, ".tar.gz") {
			writeFile(p.t, p.git.Dir, filepath.Join("dist", "linux_amd64", asset), "archive "+asset)
		}
	}
	return repo, plan
}

// repo loads the fixture repository.
func (p *publishFixture) repo() *Repo {
	p.t.Helper()
	repo, err := LoadRepo(p.git.Dir)
	assert.NilError(p.t, err)
	return repo
}

// publish runs Publish with the fixture's remote and fake forge.
func (p *publishFixture) publish(repo *Repo, plan *Plan) error {
	p.t.Helper()
	return Publish(context.Background(), repo, plan, &PublishOptions{
		Dist: "dist", Remote: p.remote, Forge: p.forge, Env: os.Environ(),
	})
}

// remoteRef returns the commit of a ref on the remote, or "".
func (p *publishFixture) remoteRef(ref string) string {
	p.t.Helper()
	out := p.run("ls-remote", "origin", ref)
	head, _, _ := strings.Cut(out, "\t")
	return head
}

func TestPublish(t *testing.T) {
	p := newPublishFixture(t, "")
	repo, plan := p.planAndBuild()
	assert.NilError(t, p.publish(repo, plan))

	// The tag and its note are on the remote.
	assert.Equal(t, p.remoteRef("refs/tags/v1.0.0"), plan.Commit)
	assert.Equal(t, p.remoteRef("refs/notes/semantic-release-v1.0.0") != "", true)
	assert.Equal(t, p.run("notes", "--ref", "semantic-release-v1.0.0", "show", "v1.0.0"), `{"channels":[null]}`)

	// The release is published with the archive and checksums.txt.
	rel := p.forge.Release("v1.0.0")
	assert.Assert(t, rel != nil && !rel.Draft)
	assert.DeepEqual(t, assetNames(rel), []string{"widgets_1.0.0_linux_amd64.tar.gz", "checksums.txt"})
	sums := string(p.forge.Uploaded["v1.0.0"]["checksums.txt"])
	assert.Assert(t, strings.HasSuffix(sums, "  widgets_1.0.0_linux_amd64.tar.gz\n"), sums)

	// The hooks ran in order with their fields; the build hook did not run.
	log, err := os.ReadFile(filepath.Join(p.git.Dir, "hooks.log"))
	assert.NilError(t, err)
	assert.Equal(t, string(log), "prepare 1.0.0 release\nsuccess v1.0.0\n")

	// Success got the commits and both releases, in publish order.
	assert.Equal(t, len(p.forge.Successes), 1)
	s := p.forge.Successes[0]
	assert.Equal(t, s.Version, "1.0.0")
	assert.Equal(t, len(s.Commits), 2)
	assert.DeepEqual(t, s.Releases, []forge.Published{
		{Name: "registry", URL: "https://registry.test/1.0.0"},
		{Name: "Fake release", URL: "https://forge.test/releases/v1.0.0"},
	})
	assert.Equal(t, len(p.forge.Failures), 0)
}

func TestPublishPrereleaseChannel(t *testing.T) {
	p := newPublishFixture(t, "")
	p.run("checkout", "-q", "main")
	p.commit("feat: next")
	p.run("push", "-q", "origin", "main")
	repo, plan := p.planAndBuild()
	assert.NilError(t, p.publish(repo, plan))
	assert.Equal(t, p.run("notes", "--ref", "semantic-release-v1.0.0-rc.1", "show", "v1.0.0-rc.1"), `{"channels":["main"]}`)
}

func TestPublishStalePlan(t *testing.T) {
	p := newPublishFixture(t, "")
	repo, plan := p.planAndBuild()

	// Another job pushes to the branch after the plan.
	p.commit("fix: later")
	p.run("push", "-q", "origin", "release")
	err := p.publish(repo, plan)
	assert.ErrorIs(t, err, ErrStalePlan)
	assert.Equal(t, p.remoteRef("refs/tags/v1.0.0"), "")
	assert.Equal(t, len(p.forge.Failures["release"]), 1, "the failure issue gets the error")
}

func TestPublishMissingAsset(t *testing.T) {
	p := newPublishFixture(t, "")
	repo, plan := p.planAndBuild()
	assert.NilError(t, os.RemoveAll(filepath.Join(p.git.Dir, "dist")))
	err := p.publish(repo, plan)
	assert.ErrorIs(t, err, ErrMissingAsset)
	assert.Equal(t, p.remoteRef("refs/tags/v1.0.0"), "")
}

func TestPublishRerunAfterUploadFailure(t *testing.T) {
	p := newPublishFixture(t, "")
	repo, plan := p.planAndBuild()
	p.forge.FailNext("UploadAsset", forge.ErrForge)
	assert.ErrorIs(t, p.publish(repo, plan), forge.ErrForge)
	rel := p.forge.Release("v1.0.0")
	assert.Assert(t, rel.Draft)
	assert.Equal(t, p.remoteRef("refs/tags/v1.0.0"), plan.Commit)

	// A broken upload is left behind, and the branch moved on meanwhile:
	// the rerun continues the same release.
	p.forge.AddBrokenAsset("v1.0.0", "widgets_1.0.0_linux_amd64.tar.gz")
	p.commit("fix: later")
	p.run("push", "-q", "origin", "release")
	assert.NilError(t, p.publish(repo, plan))
	rel = p.forge.Release("v1.0.0")
	assert.Assert(t, !rel.Draft)
	assert.DeepEqual(t, assetNames(rel), []string{"widgets_1.0.0_linux_amd64.tar.gz", "checksums.txt"})
	assert.Assert(t, rel.Assets[0].Complete)
	assert.Equal(t, p.remoteRef("refs/tags/v1.0.0"), plan.Commit)
}

func TestPublishReleaseCommit(t *testing.T) {
	p := newPublishFixture(t, `  - - "@semantic-release/git"
    - assets: [version.txt]
  - - "@semantic-release/exec"
    - prepareCmd: "echo ${nextRelease.version} > version.txt"
`)
	writeFile(t, p.git.Dir, "version.txt", "0.0.0\n")
	p.run("add", "version.txt")
	p.run("commit", "-q", "-m", "fix: add version file")
	p.run("push", "-q", "origin", "release")
	repo, plan := p.planAndBuild()

	// The prepare step runs in plugin order: the commit plugin comes
	// before the hook that writes the file, so it has nothing to commit,
	// as in semantic-release, and the tag is on the plan commit.
	assert.NilError(t, p.publish(repo, plan))
	assert.Equal(t, p.remoteRef("refs/tags/v1.0.0"), plan.Commit)
}

func TestPublishReleaseCommitAfterHook(t *testing.T) {
	p := newPublishFixture(t, `  - - "@semantic-release/exec"
    - prepareCmd: "echo ${nextRelease.version} > version.txt"
  - - "@semantic-release/git"
    - assets: [version.txt]
`)
	writeFile(t, p.git.Dir, "version.txt", "0.0.0\n")
	p.run("add", "version.txt")
	p.run("commit", "-q", "-m", "fix: add version file")
	p.run("push", "-q", "origin", "release")
	repo, plan := p.planAndBuild()
	assert.NilError(t, p.publish(repo, plan))

	// The tag and the branch are the release commit, whose parent is the
	// plan commit.
	tagged := p.remoteRef("refs/tags/v1.0.0")
	assert.Assert(t, tagged != plan.Commit)
	assert.Equal(t, p.remoteRef("refs/heads/release"), tagged)
	assert.Equal(t, p.run("log", "-1", "--format=%P", tagged), plan.Commit)
	assert.Equal(t, p.run("show", tagged+":version.txt"), "1.0.0")
	assert.Equal(t, p.run("log", "-1", "--format=%an", tagged), "semantic-release-bot")

	// A rerun from the same plan finds the release commit and continues.
	p.forge.FailNext("Success", forge.ErrForge)
	assert.NilError(t, os.Remove(filepath.Join(p.git.Dir, "dist", "checksums.txt")))
	assert.ErrorIs(t, p.publish(repo, plan), forge.ErrForge)
	assert.NilError(t, p.publish(repo, plan))
	assert.Equal(t, p.remoteRef("refs/tags/v1.0.0"), tagged)
}

func TestPublishTagOnOtherCommit(t *testing.T) {
	p := newPublishFixture(t, "")
	repo, plan := p.planAndBuild()
	p.run("tag", "v1.0.0", "HEAD~1")
	p.run("push", "-q", "origin", "v1.0.0")
	assert.ErrorIs(t, p.publish(repo, plan), ErrTagExists)
}

func TestPublishNothingToRelease(t *testing.T) {
	assert.NilError(t, Publish(context.Background(), &Repo{}, &Plan{}, &PublishOptions{}))
}

// assetNames returns the names of the assets of a release, in order.
func assetNames(r *forge.Release) []string {
	names := make([]string, 0, len(r.Assets))
	for _, a := range r.Assets {
		names = append(names, a.Name)
	}
	return names
}

func TestPublishDryRun(t *testing.T) {
	p := newPublishFixture(t, "")
	repo, plan := p.planAndBuild()
	err := Publish(context.Background(), repo, plan, &PublishOptions{
		Dist: "dist", Remote: p.remote, Forge: p.forge, Env: os.Environ(), DryRun: true,
	})
	assert.NilError(t, err)
	assert.Equal(t, p.remoteRef("refs/tags/v1.0.0"), "")
	assert.Equal(t, len(p.forge.Calls), 0)

	// A missing asset fails the dry run, and no failure issue is made.
	assert.NilError(t, os.RemoveAll(filepath.Join(p.git.Dir, "dist")))
	err = Publish(context.Background(), repo, plan, &PublishOptions{
		Dist: "dist", Remote: p.remote, Forge: p.forge, Env: os.Environ(), DryRun: true,
	})
	assert.ErrorIs(t, err, ErrMissingAsset)
	assert.Equal(t, len(p.forge.Failures), 0)
}

// gemSection is a release section with a gem that is not a client gem,
// an npm release unit, and root path rules.
const gemSection = `release:
  branches:
    - name: main
  publishers:
    - notes
    - github
    - gem:
        dir: ruby-grpc/widgets
        name: widgets
        versionFile: lib/version.rb
  root:
    exclude: [node/**]
  units:
    - kind: npm
      dir: node/widgets-client
      tagFormat: nodejs-widgets-client-v${version}
`

// gemVersionFile is the version file of the gem of gemSection.
const gemVersionFile = "module Widgets\n  VERSION = \"0.0.0\"\nend\n"

// fakeGemTools is a fake bundle and gem. Each call appends its folder,
// its arguments and the VERSION line to $WIDGETS_LOG. "bundle exec rake
// build" writes the .gem file of that version, as rake build does, and
// fails when $WIDGETS_FAIL is set.
const fakeGemTools = `#!/bin/sh
version=$(sed -n 's/.*VERSION = "\(.*\)".*/\1/p' lib/version.rb 2>/dev/null)
echo "$PWD $(basename "$0") $* $version" >> "$WIDGETS_LOG"
if [ "$1" = exec ]; then
  [ -z "$WIDGETS_FAIL" ] || { echo "rake aborted!" >&2; exit 1; }
  mkdir -p pkg && echo gem > "pkg/widgets-$(echo "$version" | sed 's/-/.pre./g').gem"
fi
`

// gemRepoFixture is a repository with gemSection, on main at v1.0.0,
// with a remote, and the fake bundle and gem on PATH.
type gemRepoFixture struct {
	*publishFixture

	// toolLog is the log of the fake tools.
	toolLog string
}

// newGemRepoFixture makes a gemRepoFixture.
func newGemRepoFixture(t *testing.T) *gemRepoFixture {
	t.Helper()
	f := newRepo(t, "main")
	f.run("remote", "add", "origin", "https://github.com/acme/widgets.git")
	writeFiles(t, f.git.Dir, map[string]string{
		native.File:                        gemSection,
		"ruby-grpc/widgets/lib/version.rb": gemVersionFile,
		"ruby-grpc/widgets/.gitignore":     "pkg/\n",
		"node/widgets-client/package.json": strings.Replace(clientPkg, "%s", "1.1.0", 1),
		"node/widgets-client/index.js":     "\n",
		"service.yaml":                     "name: widgets\n",
		"main.go":                          "package main\n",
	})
	f.run("add", ".")
	f.run("commit", "-q", "-m", "feat: init")
	f.addTag("v1.0.0", "")
	f.addTag("nodejs-widgets-client-v1.1.0", "")
	f.withRemote()
	f.run("push", "-q", "origin", "refs/notes/*")

	withFakeCLIs(t, map[string]string{"bundle": fakeGemTools, "gem": fakeGemTools})
	return &gemRepoFixture{
		publishFixture: &publishFixture{fixture: f, forge: forgetest.New()},
		toolLog:        filepath.Join(t.TempDir(), "tools.log"),
	}
}

// env returns the environment of the fake tools.
func (g *gemRepoFixture) env(extra ...string) []string {
	return append(os.Environ(), append([]string{"WIDGETS_LOG=" + g.toolLog}, extra...)...)
}

// toolCalls returns the calls of the fake tools, one per line, and
// removes the log.
func (g *gemRepoFixture) toolCalls() []string {
	g.t.Helper()
	b, err := os.ReadFile(g.toolLog)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	assert.NilError(g.t, err)
	assert.NilError(g.t, os.Remove(g.toolLog))
	return strings.Split(strings.TrimSuffix(string(b), "\n"), "\n")
}

// TestPublishDryRunBuildsGem checks that publish --dry-run builds the gem
// in the checkout with the planned version, restores the version file,
// and publishes nothing.
func TestPublishDryRunBuildsGem(t *testing.T) {
	g := newGemRepoFixture(t)
	g.commit("feat: widget")
	g.run("push", "-q", "origin", "main")
	plan, err := g.plan()
	assert.NilError(t, err)
	assert.Equal(t, plan.NextRelease.Version, "1.1.0")
	var log strings.Builder
	err = Publish(context.Background(), g.repo(), plan, &PublishOptions{
		Dist: "dist", Remote: g.remote, Forge: g.forge, Env: g.env(), Log: &log, DryRun: true,
	})
	assert.NilError(t, err)

	gemDir := filepath.Join(g.git.Dir, "ruby-grpc", "widgets")
	assert.DeepEqual(t, g.toolCalls(), []string{
		gemDir + " bundle install 1.1.0",
		gemDir + " bundle exec rake build 1.1.0",
	})
	assert.Assert(t, strings.Contains(log.String(), "Build check: widgets 1.1.0\n"), log.String())
	_, err = os.Stat(filepath.Join(gemDir, "pkg", "widgets-1.1.0.gem"))
	assert.NilError(t, err)

	// The version file is restored, and nothing is tagged, pushed or
	// published.
	assert.Equal(t, g.run("status", "--porcelain"), "")
	assert.Equal(t, g.run("tag", "--list", "v1.1.0"), "")
	assert.Equal(t, g.remoteRef("refs/tags/v1.1.0"), "")
	assert.Equal(t, g.remoteRef("refs/heads/main"), plan.Commit)
	assert.Assert(t, g.forge.Release("v1.1.0") == nil)
}
