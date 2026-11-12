// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Tests for Publish and Rollback with client publishers and
// fake client tools: a service with npm and Ruby clients (scenario 10 of
// the RFC), and the client version files of requirement 12.

package release

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/getoutreach/devbase/v2/internal/release/forge"
	"github.com/getoutreach/devbase/v2/internal/release/forge/forgetest"
	"gotest.tools/v3/assert"
)

// clientsConfig adds the npm client and a release commit of its version
// file.
const clientsConfig = `  - - "@semantic-release/npm"
    - pkgRoot: api/clients/node
  - - "@semantic-release/git"
    - assets:
        - api/clients/node/package.json
`

// fakeNPM is an npm CLI for the publish of a new version: npm version
// writes package.json, and npm view finds no version.
const fakeNPM = `#!/bin/sh
case "$1" in
version) printf '{"name": "@acme/widgets-client", "version": "%s"}\n' "$2" > package.json ;;
view) echo "npm error code E404" >&2; exit 1 ;;
esac
`

// withFakeCLIs puts scripts with the given names and contents first on
// PATH, and gives the test an empty HOME.
func withFakeCLIs(t *testing.T, scripts map[string]string) {
	t.Helper()
	bin := t.TempDir()
	for name, script := range scripts {
		assert.NilError(t, os.WriteFile(filepath.Join(bin, name), []byte(script), 0o700))
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("HOME", t.TempDir())
}

// clientsFixture makes a publish fixture with the client config and the
// committed placeholder versions.
func clientsFixture(t *testing.T, extra string, files map[string]string) *publishFixture {
	t.Helper()
	p := newPublishFixture(t, clientsConfig+extra)
	files["api/clients/node/package.json"] = `{"name": "@acme/widgets-client", "version": "0.0.0-development"}` + "\n"
	for name, content := range files {
		writeFile(t, p.git.Dir, name, content)
	}
	p.run("add", ".")
	p.run("commit", "-q", "-m", "fix: add the clients")
	p.run("push", "-q", "origin", "release")
	return p
}

func TestPublishClientsMakeNoReleaseCommit(t *testing.T) {
	withFakeCLIs(t, map[string]string{"npm": fakeNPM})
	t.Setenv("NPM_TOKEN", "npm-token-value")
	p := clientsFixture(t, "", map[string]string{})
	repo, plan := p.planAndBuild()
	assert.Equal(t, len(repo.Warnings), 1, strings.Join(repo.Warnings, "\n"))
	assert.NilError(t, p.publish(repo, plan))

	// The tag is on the plan commit, and the checkout has the version.
	assert.Equal(t, p.remoteRef("refs/tags/v1.0.0"), plan.Commit)
	assert.Equal(t, p.remoteRef("refs/heads/release"), plan.Commit)
	assert.Assert(t, strings.Contains(p.run("status", "--porcelain"), "api/clients/node/package.json"))
}

func TestPublishReleaseCommitLeavesOutClientVersionFiles(t *testing.T) {
	withFakeCLIs(t, map[string]string{"npm": fakeNPM})
	t.Setenv("NPM_TOKEN", "npm-token-value")
	// The folder glob is not a client version file, so the plan keeps it,
	// and the release commit leaves out the version files in it.
	p := clientsFixture(t, `  - - "@semantic-release/exec"
    - prepareCmd: "echo ${nextRelease.version} > api/clients/VERSION"
  - - "@semantic-release/git"
    - assets: [api/clients]
`, map[string]string{"api/clients/VERSION": "0.0.0\n"})
	repo, plan := p.planAndBuild()
	assert.NilError(t, p.publish(repo, plan))

	tagged := p.remoteRef("refs/tags/v1.0.0")
	assert.Equal(t, p.run("log", "-1", "--format=%P", tagged), plan.Commit)
	assert.Equal(t, p.run("diff", "--name-only", plan.Commit, tagged), "api/clients/VERSION")
}

// serviceReleaserc is the stencil-golang output for a service named
// widgets with npm and Ruby gRPC clients.
const serviceReleaserc = `preset: conventionalcommits
branches:
  - name: main
plugins:
  - - "@semantic-release/commit-analyzer"
    - releaseRules:
        - type: revert
          release: patch
        - type: perf
          release: patch
  - - "@semantic-release/exec"
    - generateNotesCmd: |-
        ./scripts/shell-wrapper.sh major-release-checker.js ${lastRelease.version} ${nextRelease.version}
  - "@semantic-release/release-notes-generator"
  - - "@semantic-release/npm"
    - pkgRoot: api/clients/node
  - - "@semantic-release/exec"
    - generateNotesCmd: |-
        ./scripts/shell-wrapper.sh ruby/build.sh ${nextRelease.version} 1>&2
      publishCmd: |-
        DRYRUN=${options.dryrun} ./scripts/shell-wrapper.sh ruby/publish.sh ${nextRelease.version}
  - - "@semantic-release/git"
    - assets:
        - api/clients/node/package.json
        - api/clients/ruby/lib/widgets_client/version.rb
  - "@semantic-release/github"
`

// fakeClientTools are shell scripts that stand for npm, bundle and gem.
// Each one appends its arguments to $FAKE_LOG. npm version and bundle
// exec rake build write what the real tools write; npm view finds the
// version only after npm publish.
var fakeClientTools = map[string]string{
	"npm": `echo "npm $1" >> "$FAKE_LOG"
case "$1" in
version) printf '{"name": "@getoutreach/widgets-client", "version": "%s"}\n' "$2" > package.json ;;
view) if [ -e "$FAKE_LOG.npm" ]; then echo 1.0.0; else echo "npm error code E404" >&2; exit 1; fi ;;
publish) touch "$FAKE_LOG.npm" ;;
esac
`,
	"bundle": `echo "bundle $*" >> "$FAKE_LOG"
if [ "$1" = exec ]; then
  v=$(sed -n 's/.*VERSION = "\(.*\)"/\1/p' lib/widgets_client/version.rb | sed 's/-/.pre./g')
  mkdir -p pkg && echo gem > "pkg/widgets_client-$v.gem"
fi
`,
	"gem": `echo "gem $*" >> "$FAKE_LOG"
`,
}

// serviceFixture is a repository with npm and Ruby clients, a remote, a
// fake forge, fake client tools and a fake GitHub Packages API.
type serviceFixture struct {
	*publishFixture

	api *fakePackages
	env []string
	log string
}

// newServiceFixture makes a serviceFixture on main with one commit, and
// with the config serviceReleaserc.
func newServiceFixture(t *testing.T) *serviceFixture {
	t.Helper()
	return newServiceFixtureWith(t, serviceReleaserc)
}

// newServiceFixtureWith makes a serviceFixture with the config releaserc.
func newServiceFixtureWith(t *testing.T, releaserc string) *serviceFixture {
	t.Helper()
	f := newRepo(t, "main")
	f.run("remote", "add", "origin", "https://github.com/acme/widgets.git")
	dir := f.git.Dir
	writeFile(t, dir, ".releaserc.yaml", releaserc)
	writeFile(t, dir, "service.yaml", "name: widgets\n")
	writeFile(t, dir, ".gitignore", "api/clients/ruby/pkg/\n")
	writeFile(t, dir, "api/clients/node/package.json", `{"name": "@getoutreach/widgets-client", "version": "0.0.0-development"}`+"\n")
	writeFile(t, dir, "api/clients/ruby/lib/widgets_client/version.rb", gemVersionRb)
	f.run("add", ".")
	f.run("commit", "-q", "-m", "feat: init")
	f.withRemote()

	home, bin := t.TempDir(), t.TempDir()
	writeFile(t, home, ".npmrc", githubNpmrc)
	writeFile(t, home, ".gem/credentials", gemCredentials)
	for name, script := range fakeClientTools {
		writeFile(t, bin, name, "#!/bin/sh\n"+script)
		assert.NilError(t, os.Chmod(filepath.Join(bin, name), 0o700))
	}
	c := &serviceFixture{
		publishFixture: &publishFixture{fixture: f, forge: forgetest.New()},
		api:            newFakePackages(t), log: filepath.Join(t.TempDir(), "tools.log"),
	}
	// os/exec finds a program on the PATH of the process.
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	c.env = append(os.Environ(), "HOME="+home, "FAKE_LOG="+c.log, "GITHUB_API_URL="+c.api.URL, "GH_TOKEN=gh-token")
	return c
}

// release plans and publishes the release, or a dry run of it.
func (c *serviceFixture) release(dryRun bool) (*Plan, error) {
	c.t.Helper()
	plan, err := c.plan()
	assert.NilError(c.t, err)
	assert.Assert(c.t, plan.Release, plan.Reason)
	return plan, c.publishPlan(plan, dryRun)
}

// publishPlan publishes the release of plan, or a dry run of it.
func (c *serviceFixture) publishPlan(plan *Plan, dryRun bool) error {
	c.t.Helper()
	repo, err := LoadRepo(c.git.Dir)
	assert.NilError(c.t, err)
	return Publish(context.Background(), repo, plan, &PublishOptions{
		Remote: c.remote, Forge: c.forge, Env: c.env, DryRun: dryRun,
	})
}

// toolCalls returns the calls of the fake tools, with the first two
// words of each.
func (c *serviceFixture) toolCalls() []string {
	c.t.Helper()
	b, err := os.ReadFile(c.log)
	if os.IsNotExist(err) {
		return nil
	}
	assert.NilError(c.t, err)
	var out []string
	for line := range strings.Lines(string(b)) {
		words := strings.Fields(line)
		out = append(out, strings.Join(words[:min(len(words), 3)], " "))
	}
	return out
}

// TestPublishClients is scenario 10 of the RFC: the npm and Ruby client
// versions are set in the checkout only, the packages are built and
// published in plugin order, and nothing is committed.
func TestPublishClients(t *testing.T) {
	c := newServiceFixture(t)
	plan, err := c.release(false)
	assert.NilError(t, err)
	assert.Equal(t, plan.NextRelease.Version, "1.0.0")

	// The prepare step runs npm version and builds the gem, and the
	// publish step publishes to npm, pushes the gem and makes the GitHub
	// release, in plugin order.
	assert.DeepEqual(t, c.toolCalls(), []string{
		"npm version", "bundle install", "bundle exec rake",
		"npm view", "npm publish", "gem push --key",
	})
	b, err := os.ReadFile(c.log)
	assert.NilError(t, err)
	gemFile := filepath.Join(c.git.Dir, "api", "clients", "ruby", "pkg", "widgets_client-1.0.0.gem")
	assert.Assert(t, strings.Contains(string(b), "gem push --key github --host https://rubygems.pkg.github.com/acme "+gemFile), string(b))
	assert.DeepEqual(t, c.api.paths, []string{"/orgs/acme/packages/rubygems/widgets_client/versions"})
	assert.Assert(t, !c.forge.Release("v1.0.0").Draft)

	// No release commit: the tag and the branch are the plan commit, and
	// the client version files in git keep their placeholder.
	assert.Equal(t, c.remoteRef("refs/tags/v1.0.0"), plan.Commit)
	assert.Equal(t, c.remoteRef("refs/heads/main"), plan.Commit)
	assert.Equal(t, c.run("show", "v1.0.0:api/clients/ruby/lib/widgets_client/version.rb"), strings.TrimSpace(gemVersionRb))
	assert.Assert(t, strings.Contains(c.run("show", "v1.0.0:api/clients/node/package.json"), "0.0.0-development"))
	rb, err := os.ReadFile(filepath.Join(c.git.Dir, "api", "clients", "ruby", "lib", "widgets_client", "version.rb"))
	assert.NilError(t, err)
	assert.Assert(t, strings.Contains(string(rb), `VERSION = "1.0.0"`))

	// A rerun (requirement 8) finds both packages and pushes nothing.
	c.api.versions = []string{"1.0.0"}
	assert.NilError(t, c.publishPlan(plan, false))
	calls := c.toolCalls()
	assert.DeepEqual(t, calls[6:], []string{"npm version", "bundle install", "bundle exec rake", "npm view"})
}

// TestPublishClientsFolderAsset checks that the release commit leaves
// out the client version files that a commit glob matches only when it
// runs, such as the files in a folder.
func TestPublishClientsFolderAsset(t *testing.T) {
	cfg := strings.Replace(serviceReleaserc, "        - api/clients/node/package.json\n", "        - api/clients/node\n", 1)
	assert.Assert(t, cfg != serviceReleaserc)
	c := newServiceFixtureWith(t, cfg)
	plan, err := c.release(false)
	assert.NilError(t, err)
	assert.Equal(t, c.remoteRef("refs/tags/v1.0.0"), plan.Commit)
}

func TestPublishClientsDryRun(t *testing.T) {
	c := newServiceFixture(t)
	_, err := c.release(true)
	assert.NilError(t, err)
	assert.Equal(t, len(c.toolCalls()), 0, "a dry run builds and pushes nothing")
	assert.Equal(t, len(c.api.paths), 0)
	assert.Equal(t, c.remoteRef("refs/tags/v1.0.0"), "")
}

// TestRollbackGem checks that rollback stops when the gem of the version
// exists (requirement 13), and removes a release whose gem push did not
// happen.
func TestRollbackGem(t *testing.T) {
	c := newServiceFixture(t)
	// The gem is pushed, and then the GitHub release fails.
	c.forge.FailNext("CreateRelease", forge.ErrForge)
	_, err := c.release(false)
	assert.ErrorIs(t, err, forge.ErrForge)
	assert.Assert(t, strings.Contains(strings.Join(c.toolCalls(), "\n"), "gem push"))

	c.api.versions = []string{"1.0.0"}
	opts := &RollbackOptions{Tag: "v1.0.0", Remote: c.remote, Forge: c.forge, Env: c.env}
	repo, err := LoadRepo(c.git.Dir)
	assert.NilError(t, err)
	_, err = Rollback(context.Background(), repo, opts)
	assert.ErrorIs(t, err, ErrRollbackRefused)
	assert.ErrorContains(t, err, "the GitHub Packages gem package for 1.0.0 exists")
	assert.Equal(t, c.remoteRef("refs/tags/v1.0.0") != "", true)

	// Without the packages, rollback removes the tag and its note.
	c.api.versions = nil
	c.api.status = 404
	assert.NilError(t, os.Remove(c.log+".npm"))
	deleted, err := Rollback(context.Background(), repo, opts)
	assert.NilError(t, err)
	assert.DeepEqual(t, deleted, []string{"remote refs/tags/v1.0.0", "remote refs/notes/semantic-release-v1.0.0"})
}
