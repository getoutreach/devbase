// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Tests for Publish and Rollback with client publishers and
// stubs of the client commands: npm and Python clients, a service with npm and Ruby
// clients (scenario 10 of the RFC), and the client version files of
// requirement 12.

package release

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/getoutreach/devbase/v2/internal/release/forge"
	"github.com/getoutreach/devbase/v2/internal/release/forge/forgetest"
	"github.com/getoutreach/devbase/v2/internal/release/releaserc"
	"github.com/getoutreach/gobox/pkg/exec"
	"github.com/getoutreach/gobox/pkg/exec/execstub"
	"gotest.tools/v3/assert"
)

// clientsConfig adds the npm client, a release commit of the client
// version files, and the stencil Python client hooks.
const clientsConfig = `  - - "@semantic-release/npm"
    - pkgRoot: api/clients/node
  - - "@semantic-release/git"
    - assets:
        - api/clients/node/package.json
        - api/clients/python/pyproject.toml
  - - "@semantic-release/exec"
    - generateNotesCmd: |-
        cd api/clients/python && $(mise which poetry) version ${nextRelease.version}
      publishCmd: |-
        if [ "${options.dryrun}" = "true" ]; then
          mise run publish:pyclient --environment=production --skip-publish=true
        else
          mise run publish:pyclient --environment=production --skip-publish=false
        fi
`

// fileEdit returns the result of a command that changes the file name in
// its folder with edit, which gets the content and the command
// arguments, as npm version and poetry version do.
func fileEdit(name string, edit func(content []byte, args []string) ([]byte, error)) execstub.Result {
	return execstub.Result{Func: func(_ context.Context, cmd *exec.Cmd, _ io.Reader, _, _ io.Writer) error {
		file := filepath.Join(cmd.Dir, name)
		b, err := os.ReadFile(file)
		if err != nil {
			return fmt.Errorf("read %s: %w", file, err)
		}
		if b, err = edit(b, cmd.Args); err != nil {
			return fmt.Errorf("edit %s: %w", file, err)
		}
		if err := os.WriteFile(file, b, 0o600); err != nil {
			return fmt.Errorf("write %s: %w", file, err)
		}
		return nil
	}}
}

// npmVersionWrites is the result of "npm version <version>", which
// writes the version to package.json.
func npmVersionWrites() execstub.Result {
	return fileEdit("package.json", func(b []byte, args []string) ([]byte, error) {
		var pkg map[string]any
		if err := json.Unmarshal(b, &pkg); err != nil {
			return nil, err
		}
		pkg["version"] = args[2]
		out, err := json.Marshal(pkg)
		return append(out, '\n'), err
	})
}

// pyprojectVersionRe matches the version line of pyproject.toml.
var pyprojectVersionRe = regexp.MustCompile(`(?m)^version = .*$`)

// poetryVersionWrites is the result of "poetry version <version>",
// which writes the version to pyproject.toml.
func poetryVersionWrites() execstub.Result {
	return fileEdit("pyproject.toml", func(b []byte, args []string) ([]byte, error) {
		return pyprojectVersionRe.ReplaceAll(b, []byte(`version = "`+args[2]+`"`)), nil
	})
}

// withClientEnv sets the npm token, the CodeArtifact repository and,
// in a new HOME, the box config of the client publishers.
func withClientEnv(t *testing.T) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("NPM_TOKEN", "npm-token-value")
	t.Setenv(releaserc.CodeArtifactDomainEnv, "acme")
	t.Setenv(releaserc.CodeArtifactRepositoryEnv, "acme-pypi")
	t.Setenv(releaserc.CodeArtifactRegionEnv, "eu-west-1")
	writeFile(t, os.Getenv("HOME"), filepath.Join(".outreach", ".config", "box", "box.yaml"),
		"config:\n  aws:\n    defaultAccountID: \"123456789012\"\n")
}

// clientsFixture makes a publish fixture with the client config, the
// committed placeholder versions and stubs of the client commands.
func clientsFixture(t *testing.T, extra string, files map[string]string) *publishFixture {
	t.Helper()
	withClientEnv(t)
	p := newPublishFixture(t, clientsConfig+extra)
	p.cmds = newCmdStubs(t)
	files["api/clients/node/package.json"] = `{"name": "@acme/widgets-client", "version": "0.0.0-development"}` + "\n"
	files["api/clients/python/pyproject.toml"] = "[project]\nname = \"widgets_client\"\nversion = \"0.0.0\"\n"
	for name, content := range files {
		writeFile(t, p.git.Dir, name, content)
	}
	p.run("add", ".")
	p.run("commit", "-q", "-m", "fix: add the clients")
	p.run("push", "-q", "origin", "release")
	return p
}

// expectClientsPrepare adds the commands of the prepare step of the
// clients of clientsConfig: the npm token check, and the versions in
// package.json and pyproject.toml.
func expectClientsPrepare(s *cmdStubs) {
	s.expect(cmdOK("acme\n"), npmWhoamiArgv...)
	s.expect(npmVersionWrites(), npmVersionArgv("1.0.0")...)
	s.expect(cmdOK("{}\n"), awsIdentity...)
	s.expect(misePoetry.res, misePoetry.argv...)
	s.expect(poetryVersionWrites(), "poetry", "version", "1.0.0")
}

// expectClientsPublish adds the commands of the publish step of the
// clients of clientsConfig, when the registries do not have the version.
func expectClientsPublish(s *cmdStubs, dir string) {
	s.expect(cmdFails(npmE404), npmViewArgv("@acme/widgets-client@1.0.0", officialNPMRegistry)...)
	s.expect(cmdOK(""), npmPublishArgv(filepath.Join(dir, "api", "clients", "node"), "latest", officialNPMRegistry)...)
	expectMissingVersion(s, "1.0.0")
	s.expectAll([]stubCmd{
		codeArtifactToken,
		misePoetry,
		{[]string{"poetry", "build"}, cmdOK("")},
		{[]string{"poetry", "publish", "--repository", "codeartifact"}, cmdOK("")},
	})
}

func TestPublishClientsMakeNoReleaseCommit(t *testing.T) {
	p := clientsFixture(t, "", map[string]string{})
	repo, plan := p.planAndBuild()
	assert.Equal(t, len(repo.Warnings), 2, strings.Join(repo.Warnings, "\n"))
	// The Python package is published with the account of the box config.
	expectClientsPrepare(p.cmds)
	expectClientsPublish(p.cmds, p.git.Dir)
	assert.NilError(t, p.publish(repo, plan))

	// The tag is on the plan commit, and the checkout has the versions.
	assert.Equal(t, p.remoteRef("refs/tags/v1.0.0"), plan.Commit)
	assert.Equal(t, p.remoteRef("refs/heads/release"), plan.Commit)
	status := p.run("status", "--porcelain")
	assert.Assert(t, strings.Contains(status, "api/clients/node/package.json"), status)
	assert.Assert(t, strings.Contains(status, "api/clients/python/pyproject.toml"), status)

	// A rerun skips the published packages. The Python package is built
	// again to compare its files with the files of the version.
	expectClientsPrepare(p.cmds)
	p.cmds.expect(cmdOK("1.0.0\n"), npmViewArgv("@acme/widgets-client@1.0.0", officialNPMRegistry)...)
	p.cmds.expect(cmdOK("Published\n"), describeArgs("1.0.0")...)
	wheel, sdist := "widgets_client-1.0.0-py3-none-any.whl", "widgets_client-1.0.0.tar.gz"
	p.cmds.expectAll([]stubCmd{
		{assetsArgs("1.0.0"), cmdOK(`["` + wheel + `", "` + sdist + `"]`)},
		{endpointArgs, cmdOK(pythonEndpoint + "\n")},
		codeArtifactToken,
		misePoetry,
		{[]string{"poetry", "build"}, poetryBuildWrites(wheel, sdist)},
	})
	assert.NilError(t, p.publish(repo, plan))
}

// TestPublishPythonClientDryRun checks that a dry run runs no client
// command: the stubs have none.
func TestPublishPythonClientDryRun(t *testing.T) {
	p := clientsFixture(t, "", map[string]string{})
	repo, plan := p.planAndBuild()
	assert.NilError(t, Publish(context.Background(), repo, plan, &PublishOptions{
		Dist: "dist", Remote: p.remote, Forge: p.forge, Env: os.Environ(), Exec: p.exec(), DryRun: true,
	}))
	assert.Equal(t, p.remoteRef("refs/tags/v1.0.0"), "")
}

func TestRollbackStopsForPythonPackage(t *testing.T) {
	p := clientsFixture(t, "", map[string]string{})
	expectClientsPrepare(p.cmds)
	p.partialRelease()
	view := npmViewArgv("@acme/widgets-client@1.0.0", officialNPMRegistry)

	// A Python package for the version stops the rollback. The npm check
	// verifies the token first.
	p.cmds.expect(cmdOK("acme\n"), npmWhoamiArgv...)
	p.cmds.expect(cmdFails(npmE404), view...)
	p.cmds.expect(cmdOK("Published\n"), describeArgs("1.0.0")...)
	_, err := p.rollback(&RollbackOptions{Tag: "v1.0.0"})
	assert.ErrorIs(t, err, ErrRollbackRefused)
	assert.ErrorContains(t, err, "the CodeArtifact acme-pypi package for 1.0.0 exists")
	assert.Assert(t, p.remoteRef("refs/tags/v1.0.0") != "")

	// Without it, the rollback deletes the tag.
	p.cmds.expect(cmdOK("acme\n"), npmWhoamiArgv...)
	p.cmds.expect(cmdFails(npmE404), view...)
	expectMissingVersion(p.cmds, "1.0.0")
	_, err = p.rollback(&RollbackOptions{Tag: "v1.0.0"})
	assert.NilError(t, err)
	assert.Equal(t, p.remoteRef("refs/tags/v1.0.0"), "")
}

// TestRollbackStopsForMissingRepository checks that rollback stops, and
// deletes nothing, when the CodeArtifact repository is missing.
func TestRollbackStopsForMissingRepository(t *testing.T) {
	p := clientsFixture(t, "", map[string]string{})
	expectClientsPrepare(p.cmds)
	p.partialRelease()
	p.cmds.expect(cmdOK("acme\n"), npmWhoamiArgv...)
	p.cmds.expect(cmdFails(npmE404), npmViewArgv("@acme/widgets-client@1.0.0", officialNPMRegistry)...)
	p.cmds.expect(cmdFails(codeArtifactNotFound), describeArgs("1.0.0")...)
	p.cmds.expect(cmdFails(codeArtifactNoRepository), endpointArgs...)
	_, err := p.rollback(&RollbackOptions{Tag: "v1.0.0"})
	assert.ErrorContains(t, err, "get the endpoint of CodeArtifact acme-pypi")
	assert.Assert(t, p.remoteRef("refs/tags/v1.0.0") != "")
}

func TestPublishReleaseCommitLeavesOutClientVersionFiles(t *testing.T) {
	// The folder glob is not a client version file, so the plan keeps it,
	// and the release commit leaves out the version files in it.
	p := clientsFixture(t, `  - - "@semantic-release/exec"
    - prepareCmd: "echo ${nextRelease.version} > api/clients/VERSION"
  - - "@semantic-release/git"
    - assets: [api/clients]
`, map[string]string{"api/clients/VERSION": "0.0.0\n"})
	repo, plan := p.planAndBuild()
	expectClientsPrepare(p.cmds)
	expectClientsPublish(p.cmds, p.git.Dir)
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

// serviceFixture is a repository with npm and Ruby clients, a remote, a
// fake forge, stubs of the client commands and a fake GitHub Packages
// API.
type serviceFixture struct {
	*publishFixture

	api *fakePackages
	env []string
}

// newServiceFixture makes a serviceFixture on main with one commit, and
// with the config serviceReleaserc.
func newServiceFixture(t *testing.T) *serviceFixture {
	t.Helper()
	return newServiceFixtureWith(t, serviceReleaserc)
}

// newServiceFixtureWith makes a serviceFixture with the config rc.
func newServiceFixtureWith(t *testing.T, rc string) *serviceFixture {
	t.Helper()
	f := newRepo(t, "main")
	f.run("remote", "add", "origin", "https://github.com/acme/widgets.git")
	dir := f.git.Dir
	writeFile(t, dir, ".releaserc.yaml", rc)
	writeFile(t, dir, "service.yaml", "name: widgets\n")
	writeFile(t, dir, ".gitignore", "api/clients/ruby/pkg/\n")
	writeFile(t, dir, "api/clients/node/package.json", `{"name": "@getoutreach/widgets-client", "version": "0.0.0-development"}`+"\n")
	writeFile(t, dir, "api/clients/ruby/lib/widgets_client/version.rb", gemVersionRb)
	f.run("add", ".")
	f.run("commit", "-q", "-m", "feat: init")
	f.withRemote()

	home := t.TempDir()
	writeFile(t, home, ".npmrc", githubNpmrc)
	writeFile(t, home, ".gem/credentials", gemCredentials)
	c := &serviceFixture{
		publishFixture: &publishFixture{fixture: f, forge: forgetest.New(), cmds: newCmdStubs(t)},
		api:            newFakePackages(t),
	}
	c.env = append(os.Environ(), "HOME="+home, "GITHUB_API_URL="+c.api.URL, "GH_TOKEN=gh-token")
	return c
}

// serviceRegistry is the npm registry of the service fixture.
const serviceRegistry = "https://npm.pkg.github.com/"

// expectGemBuild adds the commands of the build of the Ruby client.
func (c *serviceFixture) expectGemBuild() {
	expectGemBuild(c.cmds, rakeBuildWrites("widgets_client", "lib/widgets_client/version.rb"))
}

// expectPrepare adds the commands of the prepare step: npm version and
// the gem build.
func (c *serviceFixture) expectPrepare() {
	c.cmds.expect(npmVersionWrites(), npmVersionArgv("1.0.0")...)
	c.expectGemBuild()
}

// expectNPMView adds npm view, which finds the version when published
// is true.
func (c *serviceFixture) expectNPMView(published bool) {
	res := cmdFails(npmE404)
	if published {
		res = cmdOK("1.0.0\n")
	}
	c.cmds.expect(res, npmViewArgv("@getoutreach/widgets-client@1.0.0", serviceRegistry)...)
}

// expectPublish adds the commands of the publish step of a new version:
// npm view, npm publish and gem push.
func (c *serviceFixture) expectPublish() {
	c.expectNPMView(false)
	c.cmds.expect(cmdOK(""), npmPublishArgv(filepath.Join(c.git.Dir, "api", "clients", "node"), "latest", serviceRegistry)...)
	c.cmds.expect(cmdOK(""), "gem", "push", "--key", "github", "--host", "https://rubygems.pkg.github.com/acme",
		filepath.Join(c.git.Dir, "api", "clients", "ruby", "pkg", "widgets_client-1.0.0.gem"))
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
		Remote: c.remote, Forge: c.forge, Env: c.env, Exec: c.exec(), DryRun: dryRun,
	})
}

// TestPublishClients is scenario 10 of the RFC: the npm and Ruby client
// versions are set in the checkout only, the packages are built and
// published in plugin order, and nothing is committed.
func TestPublishClients(t *testing.T) {
	c := newServiceFixture(t)
	// The prepare step runs npm version and builds the gem, and the
	// publish step publishes to npm, pushes the gem and makes the GitHub
	// release, in plugin order.
	c.expectPrepare()
	c.expectPublish()
	plan, err := c.release(false)
	assert.NilError(t, err)
	assert.Equal(t, plan.NextRelease.Version, "1.0.0")
	gemDir := filepath.Join(c.git.Dir, "api", "clients", "ruby")
	for _, call := range c.cmds.calls()[1:3] {
		assert.Equal(t, call.Cmd.Dir, gemDir)
	}
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
	c.expectPrepare()
	c.expectNPMView(true)
	assert.NilError(t, c.publishPlan(plan, false))
}

// TestPublishClientsFolderAsset checks that the release commit leaves
// out the client version files that a commit glob matches only when it
// runs, such as the files in a folder.
func TestPublishClientsFolderAsset(t *testing.T) {
	cfg := strings.Replace(serviceReleaserc, "        - api/clients/node/package.json\n", "        - api/clients/node\n", 1)
	assert.Assert(t, cfg != serviceReleaserc)
	c := newServiceFixtureWith(t, cfg)
	c.expectPrepare()
	c.expectPublish()
	plan, err := c.release(false)
	assert.NilError(t, err)
	assert.Equal(t, c.remoteRef("refs/tags/v1.0.0"), plan.Commit)
}

func TestPublishClientsDryRun(t *testing.T) {
	c := newServiceFixture(t)
	// A dry run builds the gem, restores its version file, and pushes
	// nothing.
	c.expectGemBuild()
	_, err := c.release(true)
	assert.NilError(t, err)
	assert.Equal(t, len(c.api.paths), 0)
	assert.Equal(t, c.remoteRef("refs/tags/v1.0.0"), "")
	rb, err := os.ReadFile(filepath.Join(c.git.Dir, "api", "clients", "ruby", "lib", "widgets_client", "version.rb"))
	assert.NilError(t, err)
	assert.Equal(t, string(rb), gemVersionRb)
}

// TestRollbackGem checks that rollback stops when the gem of the version
// exists (requirement 13), and removes a release whose gem push did not
// happen.
func TestRollbackGem(t *testing.T) {
	c := newServiceFixture(t)
	// The gem is pushed, and then the GitHub release fails.
	c.forge.FailNext("CreateRelease", forge.ErrForge)
	c.expectPrepare()
	c.expectPublish()
	_, err := c.release(false)
	assert.ErrorIs(t, err, forge.ErrForge)

	c.api.versions = []string{"1.0.0"}
	c.expectNPMView(true)
	opts := &RollbackOptions{Tag: "v1.0.0", Remote: c.remote, Forge: c.forge, Env: c.env, Exec: c.exec()}
	repo, err := LoadRepo(c.git.Dir)
	assert.NilError(t, err)
	_, err = Rollback(context.Background(), repo, opts)
	assert.ErrorIs(t, err, ErrRollbackRefused)
	assert.ErrorContains(t, err, "the GitHub Packages gem package for 1.0.0 exists")
	assert.Equal(t, c.remoteRef("refs/tags/v1.0.0") != "", true)

	// Without the packages, rollback removes the tag and its note.
	c.api.versions = nil
	c.api.status = 404
	c.expectNPMView(false)
	deleted, err := Rollback(context.Background(), repo, opts)
	assert.NilError(t, err)
	assert.DeepEqual(t, deleted, []string{"remote refs/tags/v1.0.0", "remote refs/notes/semantic-release-v1.0.0"})
}
