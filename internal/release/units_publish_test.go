// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Tests for the publish and the rollback of release units,
// with a bare remote, a fake forge and stubs of the npm commands.

package release

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/getoutreach/devbase/v2/internal/release/forge"
	"github.com/getoutreach/devbase/v2/internal/release/spec"
	"github.com/getoutreach/gobox/pkg/exec/execstub"
	"gotest.tools/v3/assert"
)

// unitRegistry is the registry of the npm package of the unit.
const unitRegistry = "https://npm.pkg.github.com"

// withNPMStubs sets NPM_TOKEN, and gives u stubs of the npm commands.
func (u *unitFixture) withNPMStubs() {
	u.t.Helper()
	u.t.Setenv("NPM_TOKEN", "test-token")
	u.cmds = newCmdStubs(u.t)
}

// expectNPMRelease adds the npm commands of the release of the unit
// with version: npm version, then npm view, which gives view, and then
// npm publish, which gives publish, when publish is not nil.
func (u *unitFixture) expectNPMRelease(version string, view execstub.Result, publish *execstub.Result) {
	u.cmds.expect(cmdOK(""), npmVersionArgv(version)...)
	u.cmds.expect(view, npmViewArgv("@acme/widgets-client@"+version, unitRegistry)...)
	if publish != nil {
		u.cmds.expect(*publish, npmPublishArgv(filepath.Join(u.git.Dir, "node", "widgets-client"), "latest", unitRegistry)...)
	}
}

// publishUnits publishes plan with the fixture's units and stubs.
func (u *unitFixture) publishUnits(plan *Plan) error {
	u.t.Helper()
	return Publish(context.Background(), u.unitRepo(), plan, &PublishOptions{
		Dist: "dist", Remote: u.remote, Forge: u.forge, Env: os.Environ(), Exec: u.exec(),
	})
}

func TestPublishUnits(t *testing.T) {
	u := baselined(t, unitsConfig)
	u.withNPMStubs()
	u.units[0].ForgeRelease = true
	u.change("feat: both", "main.go", "node/widgets-client/index.js")
	plan, err := u.planUnits()
	assert.NilError(t, err)
	u.expectNPMRelease("1.2.0", cmdFails(npmE404), &execstub.Result{})
	assert.NilError(t, u.publishUnits(plan))

	// Both tags are on the plan commit, with their notes.
	assert.Equal(t, u.remoteRef("refs/tags/v1.1.0"), plan.Commit)
	assert.Equal(t, u.remoteRef("refs/tags/nodejs-widgets-client-v1.2.0"), plan.Commit)
	assert.Assert(t, u.remoteRef("refs/notes/semantic-release-nodejs-widgets-client-v1.2.0") != "")

	// npm set the version in node/widgets-client and published it.
	assert.Equal(t, u.cmds.calls()[0].Cmd.Dir, filepath.Join(u.git.Dir, "node", "widgets-client"))

	// The root release is the latest one; the unit release, which comes
	// first, is not.
	assert.Equal(t, len(u.forge.Created), 2)
	unit, root := u.forge.Created[0], u.forge.Created[1]
	assert.Equal(t, root.Tag, "v1.1.0")
	assert.Assert(t, root.Latest)
	assert.Equal(t, unit.Tag, "nodejs-widgets-client-v1.2.0")
	assert.Equal(t, unit.Name, "@acme/widgets-client v1.2.0")
	assert.Assert(t, !unit.Latest)
	assert.Assert(t, strings.Contains(unit.Notes, "compare/nodejs-widgets-client-v1.1.0...nodejs-widgets-client-v1.2.0"), unit.Notes)

	// Each release has its success step; the unit's names the package.
	assert.Equal(t, len(u.forge.Successes), 2)
	s := u.forge.Successes[1]
	assert.Equal(t, s.Unit, "@acme/widgets-client")
	assert.Equal(t, s.Version, "1.2.0")
	assert.Equal(t, len(s.Commits), 1)
	assert.Equal(t, s.Commits[0].Message, "feat: both")
	assert.DeepEqual(t, s.Releases, []forge.Published{
		{Name: "npm package (@latest dist-tag)"},
		{Name: "Fake release", URL: "https://forge.test/releases/nodejs-widgets-client-v1.2.0"},
	})
	assert.Equal(t, u.forge.Successes[0].Unit, "")

	// The root success step has only the root commits.
	assert.Equal(t, len(u.forge.Successes[0].Commits), 1)
}

func TestPublishUnitInstall(t *testing.T) {
	u := baselined(t, unitsConfig)
	u.withNPMStubs()
	u.units[0].NPM = &spec.NPMPlugin{Install: "yarn install --frozen-lockfile"}
	u.change("fix: client", "node/widgets-client/index.js")
	plan, err := u.planUnits()
	assert.NilError(t, err)

	// A dry run does not install: the stubs have no commands yet.
	err = Publish(context.Background(), u.unitRepo(), plan, &PublishOptions{
		Remote: u.remote, Forge: u.forge, Env: os.Environ(), Exec: u.exec(), DryRun: true,
	})
	assert.NilError(t, err)

	// The install runs in the unit folder before npm version.
	u.cmds.expect(cmdOK(""), "yarn", "install", "--frozen-lockfile")
	u.expectNPMRelease("1.1.1", cmdFails(npmE404), &execstub.Result{})
	assert.NilError(t, u.publishUnits(plan))
	assert.Equal(t, u.cmds.calls()[0].Cmd.Dir, filepath.Join(u.git.Dir, "node", "widgets-client"))
}

func TestPublishUnitOnly(t *testing.T) {
	u := baselined(t, unitsConfig)
	u.withNPMStubs()
	u.change("fix: client", "node/widgets-client/index.js")
	plan, err := u.planUnits()
	assert.NilError(t, err)
	assert.Assert(t, plan.Release && !plan.RootRelease())
	u.expectNPMRelease("1.1.1", cmdFails(npmE404), &execstub.Result{})
	assert.NilError(t, u.publishUnits(plan))
	assert.Equal(t, u.remoteRef("refs/tags/nodejs-widgets-client-v1.1.1"), plan.Commit)
	assert.Equal(t, u.remoteRef("refs/tags/v1.0.1"), "")
	assert.Equal(t, len(u.forge.Created), 0)
	assert.Equal(t, len(u.forge.Successes), 1)
}

func TestPublishUnitsRerun(t *testing.T) {
	u := baselined(t, unitsConfig)
	u.withNPMStubs()
	u.change("feat: both", "main.go", "node/widgets-client/index.js")
	plan, err := u.planUnits()
	assert.NilError(t, err)

	// The npm publish fails: the root release goes on, and publish fails
	// at the end with the unit error.
	u.expectNPMRelease("1.2.0", cmdFails(npmE404), &execstub.Result{Stderr: "npm error boom\n", ExitCode: 1})
	err = u.publishUnits(plan)
	assert.ErrorContains(t, err, "unit widgets-client: npm publish: npm publish ")
	assert.ErrorContains(t, err, "exit status 1: npm error boom")
	assert.Equal(t, u.remoteRef("refs/tags/v1.1.0"), plan.Commit)
	assert.Equal(t, u.remoteRef("refs/tags/nodejs-widgets-client-v1.2.0"), plan.Commit)
	assert.Equal(t, len(u.forge.Successes), 1, "only the root release succeeded")
	assert.Equal(t, len(u.forge.Failures["main"]), 1)

	// The branch moved on; the rerun continues the same release.
	u.change("fix: later", "main.go")
	u.expectNPMRelease("1.2.0", cmdFails(npmE404), &execstub.Result{})
	var log bytes.Buffer
	err = Publish(context.Background(), u.unitRepo(), plan, &PublishOptions{
		Remote: u.remote, Forge: u.forge, Env: os.Environ(), Exec: u.exec(), Log: &log,
	})
	assert.NilError(t, err)
	assert.Assert(t, strings.Contains(log.String(), "Continue the release of nodejs-widgets-client-v1.2.0"), log.String())
	assert.Equal(t, len(u.forge.Successes), 3)

	// A third run finds the package and publishes nothing again.
	u.expectNPMRelease("1.2.0", cmdOK("1.2.0\n"), nil)
	assert.NilError(t, u.publishUnits(plan))
}

func TestPublishUnitsStale(t *testing.T) {
	u := baselined(t, unitsConfig)
	u.withNPMStubs()
	u.change("fix: client", "node/widgets-client/index.js")
	plan, err := u.planUnits()
	assert.NilError(t, err)
	u.change("fix: later", "main.go")
	assert.ErrorIs(t, u.publishUnits(plan), ErrStalePlan)
	assert.Equal(t, u.remoteRef("refs/tags/nodejs-widgets-client-v1.1.1"), "")
}

func TestPublishUnitTagElsewhere(t *testing.T) {
	u := baselined(t, unitsConfig)
	u.withNPMStubs()
	u.change("fix: client", "node/widgets-client/index.js")
	plan, err := u.planUnits()
	assert.NilError(t, err)
	u.run("tag", "nodejs-widgets-client-v1.1.1", "HEAD~1")
	assert.ErrorIs(t, u.publishUnits(plan), ErrTagExists)
}

func TestPublishGoPin(t *testing.T) {
	cfg := unitsConfig + `  - - "@semantic-release/git"
    - assets: [go.mod, go.sum]
`
	u := goFixture(t, cfg, &spec.Unit{Kind: spec.UnitGoModule, Dir: "api"})
	u.change("fix: api", "api/api.go")
	plan, err := u.planUnits()
	assert.NilError(t, err)
	assert.Equal(t, versions(plan), "root=1.0.1 widgets-client=- api=1.0.1")
	assert.NilError(t, u.publishUnits(plan))

	// The unit tag is on the plan commit; the root tag is on the release
	// commit, whose go.mod requires the new unit version.
	assert.Equal(t, u.remoteRef("refs/tags/api/v1.0.1"), plan.Commit)
	tagged := u.remoteRef("refs/tags/v1.0.1")
	assert.Equal(t, u.run("log", "-1", "--format=%P", tagged), plan.Commit)
	assert.Assert(t, strings.Contains(u.run("show", tagged+":go.mod"), "github.com/acme/widgets/api v1.0.1"))
	assert.Equal(t, u.remoteRef("refs/heads/main"), tagged)

	// The Go unit has no forge release.
	assert.Equal(t, len(u.forge.Created), 1)
}

func TestRollbackUnit(t *testing.T) {
	u := baselined(t, unitsConfig)
	u.withNPMStubs()
	u.units[0].ForgeRelease = true
	u.change("feat: both", "main.go", "node/widgets-client/index.js")
	plan, err := u.planUnits()
	assert.NilError(t, err)
	u.expectNPMRelease("1.2.0", cmdFails(npmE404), &execstub.Result{})
	assert.NilError(t, u.publishUnits(plan))

	rollback := func(tag string, force bool) ([]string, string, error) {
		var log bytes.Buffer
		deleted, err := Rollback(context.Background(), u.unitRepo(), &RollbackOptions{
			Tag: tag, Remote: u.remote, Forge: u.forge, Env: os.Environ(), Exec: u.exec(), Force: force, Log: &log,
		})
		return deleted, log.String(), err
	}
	view := npmViewArgv("@acme/widgets-client@1.2.0", unitRegistry)

	// The package exists and the release is published.
	u.cmds.expect(cmdOK("1.2.0\n"), view...)
	_, _, err = rollback("nodejs-widgets-client-v1.2.0", false)
	assert.ErrorIs(t, err, ErrRollbackRefused)
	assert.ErrorContains(t, err, "the npm package for 1.2.0 exists")
	assert.ErrorContains(t, err, "is published")

	// Without the package, --force removes the unit release only.
	u.cmds.expect(cmdFails(npmE404), view...)
	deleted, _, err := rollback("nodejs-widgets-client-v1.2.0", true)
	assert.NilError(t, err)
	assert.DeepEqual(t, deleted, []string{
		"Fake release https://forge.test/releases/nodejs-widgets-client-v1.2.0",
		"remote refs/tags/nodejs-widgets-client-v1.2.0",
		"remote refs/notes/semantic-release-nodejs-widgets-client-v1.2.0",
	})
	assert.Equal(t, u.remoteRef("refs/tags/nodejs-widgets-client-v1.2.0"), "")
	assert.Equal(t, u.remoteRef("refs/tags/v1.1.0"), plan.Commit)
	assert.Assert(t, u.forge.Release("v1.1.0") != nil)

	// A rerun of the plan releases the unit again.
	u.expectNPMRelease("1.2.0", cmdFails(npmE404), &execstub.Result{})
	assert.NilError(t, u.publishUnits(plan))
	assert.Equal(t, u.remoteRef("refs/tags/nodejs-widgets-client-v1.2.0"), plan.Commit)

	// A root rollback names the unit tags that stay.
	_, log, err := rollback("v1.1.0", true)
	assert.NilError(t, err)
	assert.Assert(t, strings.Contains(log, "The unit tags nodejs-widgets-client-v1.2.0 stay"), log)

	_, _, err = rollback("nodejs-other-v1.0.0", false)
	assert.ErrorContains(t, err, "is not a release tag")
}

func TestRollbackGoUnit(t *testing.T) {
	u := goFixture(t, unitsConfig, &spec.Unit{Kind: spec.UnitGoModule, Dir: "api", Go: &spec.GoUnit{PinRoot: spec.GoPinNever}})
	u.change("fix: api", "api/api.go")
	plan, err := u.planUnits()
	assert.NilError(t, err)
	assert.NilError(t, u.publishUnits(plan))
	opts := &RollbackOptions{Tag: "api/v1.0.1", Remote: u.remote, Forge: u.forge, Env: os.Environ()}
	_, err = Rollback(context.Background(), u.unitRepo(), opts)
	assert.ErrorContains(t, err, "Go treats a version as fixed")
	opts.Force = true
	deleted, err := Rollback(context.Background(), u.unitRepo(), opts)
	assert.NilError(t, err)
	assert.DeepEqual(t, deleted, []string{"remote refs/tags/api/v1.0.1", "remote refs/notes/semantic-release-api/v1.0.1"})
}
