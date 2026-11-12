// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Tests for the release units: commit attribution, the unit
// plan, the baseline command and the plan file schema 2.

package release

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/getoutreach/devbase/v2/internal/release/forge/forgetest"
	"github.com/getoutreach/devbase/v2/internal/release/forge/github"
	"github.com/getoutreach/devbase/v2/internal/release/spec"
	"gotest.tools/v3/assert"
)

// unitsConfig releases from main only and makes GitHub releases.
const unitsConfig = `preset: conventionalcommits
branches:
  - name: main
plugins:
  - "@semantic-release/commit-analyzer"
  - "@semantic-release/release-notes-generator"
  - "@semantic-release/github"
`

// clientPkg is the package.json of the npm package of the fixtures.
const clientPkg = `{"name": "@acme/widgets-client", "version": "%s", "publishConfig": {"registry": "https://npm.pkg.github.com"}}`

// exampleRoot are the root path rules of a Go library with npm
// packages in node/.
var exampleRoot = spec.RootPaths{
	Exclude: []string{"node/**", "scripts/check-package.js"},
	Shared:  []string{".circleci/config.yml", ".tool-versions"},
}

// nodeUnit returns an npm unit for node/<dir> with the tag format
// nodejs-<dir>-v${version}.
func nodeUnit(dir string) spec.Unit {
	return spec.Unit{Kind: spec.UnitNPM, Dir: "node/" + dir, TagFormat: spec.TagFormat("nodejs-" + dir + "-v${version}")}
}

func TestAttribution(t *testing.T) {
	units := []spec.Unit{nodeUnit("widgets-client"), nodeUnit("rpc-client"), nodeUnit("lint-config")}
	for i := range units {
		units[i].SetDefaults()
	}
	a := &attribution{units: units, root: exampleRoot}
	cases := []struct {
		name  string
		files []string
		root  bool
		units []string
	}{
		{"package and shared file", []string{"node/lint-config/node.json", ".circleci/config.yml"}, false, []string{"lint-config"}},
		{
			"7157be8",
			[]string{"node/widgets-client/x", "node/rpc-client/y", ".circleci/config.yml", ".tool-versions"},
			false,
			[]string{"widgets-client", "rpc-client"},
		},
		{"package only", []string{"node/widgets-client/src/index.ts"}, false, []string{"widgets-client"}},
		{"package and go.mod", []string{"node/widgets-client/yarn.lock", "go.mod", "go.sum"}, true, []string{"widgets-client"}},
		{"package and Go code", []string{"node/widgets-client/src/a.ts", "pkg/widgets/a.go"}, true, []string{"widgets-client"}},
		{"shared only", []string{".circleci/config.yml"}, true, nil},
		{"docs only", []string{"node/README.md"}, false, nil},
		{"empty", nil, true, nil},
	}
	for _, tc := range cases {
		assert.Equal(t, a.rootGets(tc.files), tc.root, tc.name)
		var got []string
		for i := range units {
			if a.unitGets(&units[i], tc.files) {
				got = append(got, units[i].Name)
			}
		}
		assert.DeepEqual(t, got, tc.units)
	}

	// Without root rules, the root gets every commit.
	a.root = spec.RootPaths{}
	assert.Assert(t, a.rootGets([]string{"node/widgets-client/x"}))

	// A nested unit's files do not belong to the outer unit.
	outer := spec.Unit{Name: "api", Paths: []string{"api"}, Dir: "api"}
	inner := spec.Unit{Name: "node", Paths: []string{"api/clients/node"}, Dir: "api/clients/node"}
	a = &attribution{units: []spec.Unit{outer, inner}}
	assert.Assert(t, !a.unitGets(&a.units[0], []string{"api/clients/node/package.json"}))
	assert.Assert(t, a.unitGets(&a.units[1], []string{"api/clients/node/package.json"}))
	assert.Assert(t, a.unitGets(&a.units[0], []string{"api/api.go"}))
}

// unitFixture is a repository with a Go root, the npm package node/widgets-client,
// shared CI files, and a bare remote.
type unitFixture struct {
	*publishFixture

	units []spec.Unit
	root  spec.RootPaths
}

// newUnitFixture makes a unitFixture on main with the root release
// v1.0.0, with config cfg and one npm unit.
func newUnitFixture(t *testing.T, cfg string) *unitFixture {
	t.Helper()
	f := newRepo(t, "main")
	f.run("remote", "add", "origin", "https://github.com/acme/widgets.git")
	writeFile(t, f.git.Dir, ".releaserc.yaml", cfg)
	writeFile(t, f.git.Dir, "main.go", "package main\n")
	writeFile(t, f.git.Dir, ".circleci/config.yml", "version: 2.1\n")
	writeFile(t, f.git.Dir, "node/widgets-client/package.json", strings.Replace(clientPkg, "%s", "1.1.0", 1))
	writeFile(t, f.git.Dir, ".gitignore", "dist/\n")
	f.run("add", ".")
	f.run("commit", "-q", "-m", "feat: init")
	f.addTag("v1.0.0", "")
	f.withRemote()
	f.run("push", "-q", "origin", "refs/notes/*")
	return &unitFixture{
		publishFixture: &publishFixture{fixture: f, forge: forgetest.New()},
		units:          []spec.Unit{nodeUnit("widgets-client")},
		root:           exampleRoot,
	}
}

// change writes files and commits them with msg, and pushes the branch.
func (u *unitFixture) change(msg string, files ...string) {
	u.t.Helper()
	for _, f := range files {
		p := filepath.Join(u.git.Dir, f)
		b, _ := os.ReadFile(p)
		writeFile(u.t, u.git.Dir, f, string(b)+msg+"\n")
	}
	u.run("add", ".")
	u.run("commit", "-q", "-m", msg)
	u.run("push", "-q", "origin", "HEAD")
}

// unitRepo loads the repository with the fixture's units.
func (u *unitFixture) unitRepo() *Repo {
	u.t.Helper()
	repo, err := LoadRepo(u.git.Dir)
	assert.NilError(u.t, err)
	assert.NilError(u.t, repo.SetUnits(u.units, u.root))
	return repo
}

// planUnits plans with the fixture's units.
func (u *unitFixture) planUnits(names ...string) (*Plan, error) {
	u.t.Helper()
	return MakePlan(context.Background(), u.unitRepo(), &PlanOptions{
		NewLinks: github.NewLinks, Remote: u.remote, Units: names,
	})
}

// baseline runs Baseline on main with the fixture's remote.
func (u *unitFixture) baseline(opts *BaselineOptions) ([]string, error) {
	u.t.Helper()
	opts.Branch, opts.Remote = "main", u.remote
	return Baseline(context.Background(), u.unitRepo(), opts)
}

// tagPlan tags the plan's releases as publish would.
func (u *unitFixture) tagPlan(plan *Plan) {
	u.t.Helper()
	if plan.RootRelease() {
		u.addTag(plan.NextRelease.GitTag, plan.NextRelease.Channel)
	}
	for _, pu := range plan.ReleasedUnits() {
		u.addTag(pu.NextRelease.GitTag, pu.NextRelease.Channel)
	}
	u.run("push", "-q", "origin", "--tags", "refs/notes/*")
}

// versions returns the next root version and the unit versions of a
// plan, with "-" for no release.
func versions(plan *Plan) string {
	out := make([]string, 0, 1+len(plan.Units))
	out = append(out, "root=-")
	if plan.RootRelease() {
		out[0] = "root=" + plan.NextRelease.Version
	}
	for _, pu := range plan.Units {
		v := "-"
		if pu.Release {
			v = pu.NextRelease.Version
		}
		out = append(out, pu.Name+"="+v)
	}
	return strings.Join(out, " ")
}

func TestPlanUnitsNeedBaseline(t *testing.T) {
	u := newUnitFixture(t, unitsConfig)

	// No tag: the plan stops and says how to make the baseline.
	_, err := u.planUnits()
	assert.ErrorIs(t, err, ErrUnitNoTag)
	assert.ErrorContains(t, err, `run "devbase release baseline" on main to tag nodejs-widgets-client-v1.1.0`)

	// A placeholder version cannot be the baseline.
	writeFile(t, u.git.Dir, "node/widgets-client/package.json", strings.Replace(clientPkg, "%s", "0.0.0-development", 1))
	u.change("chore: placeholder")
	_, err = u.planUnits()
	assert.ErrorIs(t, err, ErrUnitNoTag)
	assert.ErrorContains(t, err, `"0.0.0-development" in node/widgets-client/package.json is not a released version`)
	_, err = u.baseline(&BaselineOptions{})
	assert.ErrorIs(t, err, ErrBaseline)
	writeFile(t, u.git.Dir, "node/widgets-client/package.json", strings.Replace(clientPkg, "%s", "not-semver", 1))
	u.change("chore: bad version")
	_, err = u.planUnits()
	assert.ErrorContains(t, err, "is not a released version")

	// The baseline tags the manifest version at HEAD, and the first run
	// releases nothing.
	writeFile(t, u.git.Dir, "node/widgets-client/package.json", strings.Replace(clientPkg, "%s", "1.1.0", 1))
	u.change("chore: real version")
	tags, err := u.baseline(&BaselineOptions{DryRun: true})
	assert.NilError(t, err)
	assert.DeepEqual(t, tags, []string{"nodejs-widgets-client-v1.1.0"})
	assert.Equal(t, u.remoteRef("refs/tags/nodejs-widgets-client-v1.1.0"), "")
	_, err = u.baseline(&BaselineOptions{})
	assert.NilError(t, err)
	head := u.run("rev-parse", "HEAD")
	assert.Equal(t, u.remoteRef("refs/tags/nodejs-widgets-client-v1.1.0"), head)
	assert.Equal(t, u.run("notes", "--ref", "semantic-release-nodejs-widgets-client-v1.1.0", "show", "nodejs-widgets-client-v1.1.0"),
		`{"channels":[null]}`)
	plan, err := u.planUnits()
	assert.NilError(t, err)
	assert.Equal(t, versions(plan), "root=- widgets-client=-")
	assert.Assert(t, !plan.Release)

	// A second baseline does nothing.
	tags, err = u.baseline(&BaselineOptions{})
	assert.NilError(t, err)
	assert.Equal(t, len(tags), 0)

	// With a tag, a placeholder manifest version does not count.
	writeFile(t, u.git.Dir, "node/widgets-client/package.json", strings.Replace(clientPkg, "%s", "0.0.0-development", 1))
	u.change("fix(client): placeholder version")
	plan, err = u.planUnits()
	assert.NilError(t, err)
	assert.Equal(t, versions(plan), "root=- widgets-client=1.1.1")
}

func TestBaselineChecks(t *testing.T) {
	u := newUnitFixture(t, unitsConfig)
	u.units = append(u.units, spec.Unit{Kind: spec.UnitGeneric, Dir: "docs", TagFormat: "docs-v${version}"})

	// HEAD must be the remote branch head.
	u.commit("chore: local")
	_, err := u.baseline(&BaselineOptions{})
	assert.ErrorIs(t, err, ErrBaseline)
	u.run("push", "-q", "origin", "main")

	// A unit without a manifest needs --version, with one --unit.
	_, err = u.baseline(&BaselineOptions{})
	assert.ErrorContains(t, err, "--unit docs --version")
	_, err = u.baseline(&BaselineOptions{Version: "1.0.0"})
	assert.ErrorIs(t, err, ErrBaseline)
	_, err = u.baseline(&BaselineOptions{Units: []string{"widgets-client"}, Version: "1.0.0"})
	assert.ErrorContains(t, err, "uses the version in its package.json")
	_, err = u.baseline(&BaselineOptions{Units: []string{"nope"}})
	assert.ErrorIs(t, err, ErrUnknownUnit)
	tags, err := u.baseline(&BaselineOptions{Units: []string{"docs"}, Version: "3.1.0"})
	assert.NilError(t, err)
	assert.DeepEqual(t, tags, []string{"docs-v3.1.0"})
	tags, err = u.baseline(&BaselineOptions{})
	assert.NilError(t, err)
	assert.DeepEqual(t, tags, []string{"nodejs-widgets-client-v1.1.0"})
}

// baselined returns a unit fixture whose units have their baseline tags.
func baselined(t *testing.T, cfg string) *unitFixture {
	t.Helper()
	u := newUnitFixture(t, cfg)
	_, err := u.baseline(&BaselineOptions{})
	assert.NilError(t, err)
	return u
}

func TestPlanUnitsByPaths(t *testing.T) {
	u := baselined(t, unitsConfig)

	// A Node-only commit releases only the package, not the root.
	u.change("feat(client): a", "node/widgets-client/index.js")
	plan, err := u.planUnits()
	assert.NilError(t, err)
	assert.Equal(t, versions(plan), "root=- widgets-client=1.2.0")
	assert.Assert(t, plan.Release)
	assert.Equal(t, plan.Reason, "no relevant changes")
	assert.Equal(t, plan.Schema, PlanSchemaUnits)
	pu := plan.Units[0]
	assert.DeepEqual(t, pu.LastRelease, &PlanRelease{Version: "1.1.0", GitTag: "nodejs-widgets-client-v1.1.0"})
	assert.DeepEqual(t, pu.NextRelease, &PlanRelease{Type: spec.BumpMinor, Version: "1.2.0", GitTag: "nodejs-widgets-client-v1.2.0"})
	assert.Assert(t, strings.Contains(pu.Notes,
		"compare/nodejs-widgets-client-v1.1.0...nodejs-widgets-client-v1.2.0"), pu.Notes)
	assert.Equal(t, len(pu.Commits), 1)
	u.tagPlan(plan)

	// A shared file with a Node change: only the package.
	u.change("feat: ci and client", ".circleci/config.yml", "node/widgets-client/index.js")
	plan, err = u.planUnits()
	assert.NilError(t, err)
	assert.Equal(t, versions(plan), "root=- widgets-client=1.3.0")
	u.tagPlan(plan)

	// A shared file alone: the root.
	u.change("fix: ci only", ".circleci/config.yml")
	plan, err = u.planUnits()
	assert.NilError(t, err)
	assert.Equal(t, versions(plan), "root=1.0.1 widgets-client=-")
	assert.Equal(t, plan.Units[0].Reason, "no relevant changes in node/widgets-client")
	u.tagPlan(plan)

	// A commit that changes the root and the package: both.
	u.change("feat: both", "main.go", "node/widgets-client/index.js")
	plan, err = u.planUnits()
	assert.NilError(t, err)
	assert.Equal(t, versions(plan), "root=1.1.0 widgets-client=1.4.0")
	assert.Assert(t, !strings.Contains(plan.Notes, "ci only"), "the root notes start at v1.0.1")
	u.tagPlan(plan)

	// --unit limits the plan.
	u.change("fix: x", "node/widgets-client/index.js")
	_, err = u.planUnits("nope")
	assert.ErrorIs(t, err, ErrUnknownUnit)
	u.units = append(u.units, spec.Unit{Kind: spec.UnitGeneric, Dir: "docs", TagFormat: "docs-v${version}"})
	plan, err = u.planUnits("widgets-client")
	assert.NilError(t, err)
	assert.Equal(t, versions(plan), "root=- widgets-client=1.4.1")
}

func TestPlanUnitsPrerelease(t *testing.T) {
	u := baselined(t, fixtureConfig+"  - \"@semantic-release/github\"\n")
	u.run("checkout", "-q", "-b", "release")
	u.run("push", "-q", "origin", "release")
	u.run("checkout", "-q", "main")
	u.units = append(u.units, spec.Unit{
		Kind: spec.UnitGeneric, Dir: "docs", TagFormat: "docs-v${version}", SkipPrereleases: true,
	})
	u.root.Exclude = append(u.root.Exclude, "docs/**")
	_, err := u.baseline(&BaselineOptions{Units: []string{"docs"}, Version: "1.0.0"})
	assert.NilError(t, err)

	u.change("feat: client and docs", "node/widgets-client/index.js", "docs/a.md")
	plan, err := u.planUnits()
	assert.NilError(t, err)
	assert.Equal(t, versions(plan), "root=- widgets-client=1.2.0-rc.1 docs=-")
	assert.Equal(t, plan.Units[0].NextRelease.Channel, "main")
	assert.Equal(t, plan.Units[1].Reason, "the unit releases only from the release branch")
	u.tagPlan(plan)
	u.change("fix: client", "node/widgets-client/index.js")
	plan, err = u.planUnits()
	assert.NilError(t, err)
	assert.Equal(t, versions(plan), "root=- widgets-client=1.2.0-rc.2 docs=-")
	u.tagPlan(plan)

	// The release branch gets the stable version of both units.
	u.run("checkout", "-q", "release")
	u.run("merge", "-q", "--ff-only", "main")
	u.run("push", "-q", "origin", "release")
	plan, err = u.planUnits()
	assert.NilError(t, err)
	assert.Equal(t, versions(plan), "root=- widgets-client=1.2.0 docs=1.1.0")
}

// goFixture adds the Go module api to a unit fixture: the root go.mod
// requires it with a local replace directive.
func goFixture(t *testing.T, cfg string, unit *spec.Unit) *unitFixture {
	t.Helper()
	u := baselined(t, cfg)
	writeFile(t, u.git.Dir, "go.mod", "module github.com/acme/widgets\n\ngo 1.22\n\n"+
		"require github.com/acme/widgets/api v0.0.0\n\nreplace github.com/acme/widgets/api => ./api\n")
	writeFile(t, u.git.Dir, "api/go.mod", "module github.com/acme/widgets/api\n\ngo 1.22\n")
	u.change("chore: add api")
	u.units = append(u.units, *unit)
	return u
}

func TestPlanGoMirror(t *testing.T) {
	u := goFixture(t, unitsConfig, &spec.Unit{Kind: spec.UnitGoModule, Dir: "api", Go: &spec.GoUnit{PinRoot: spec.GoPinNever}})

	// A change in api releases the root and api with the root version.
	u.change("fix: api", "api/api.go")
	plan, err := u.planUnits()
	assert.NilError(t, err)
	assert.Equal(t, versions(plan), "root=1.0.1 widgets-client=- api=1.0.1")
	assert.Equal(t, plan.Units[1].NextRelease.GitTag, "api/v1.0.1")
	assert.Equal(t, plan.Units[1].Strategy, spec.StrategyMirror)
	u.tagPlan(plan)

	// A root change does not tag api; a chore in api does not either.
	u.change("feat: root", "main.go")
	u.change("chore: api", "api/api.go")
	plan, err = u.planUnits()
	assert.NilError(t, err)
	assert.Equal(t, versions(plan), "root=1.1.0 widgets-client=- api=-")
	u.tagPlan(plan)

	// A manual unit tag above the root version stops the plan.
	u.run("tag", "api/v9.0.0")
	u.change("fix: api again", "api/api.go")
	_, err = u.planUnits()
	assert.ErrorIs(t, err, ErrUnitVersion)
}

func TestPlanGoMajorSuffix(t *testing.T) {
	u := goFixture(t, unitsConfig, &spec.Unit{
		Kind: spec.UnitGoModule, Dir: "api", Strategy: spec.StrategyIndependent, Go: &spec.GoUnit{PinRoot: spec.GoPinNever},
	})
	_, err := u.planUnits()
	assert.ErrorIs(t, err, ErrUnitNoTag)
	assert.ErrorContains(t, err, "--unit api --version")
	_, err = u.baseline(&BaselineOptions{Units: []string{"api"}, Version: "2.0.0"})
	assert.ErrorIs(t, err, spec.ErrUnsupported, "v2 needs the /v2 module path")
	_, err = u.baseline(&BaselineOptions{Units: []string{"api"}, Version: "1.4.0"})
	assert.NilError(t, err)

	// A breaking change without the /v2 module path stops the plan.
	u.change("feat(api)!: break", "api/api.go")
	_, err = u.planUnits()
	assert.ErrorIs(t, err, spec.ErrUnsupported)
	assert.ErrorContains(t, err, `needs the module path suffix "/v2"`)

	// With the /v2 path (major branch layout), the tag is api/v2.0.0.
	writeFile(t, u.git.Dir, "api/go.mod", "module github.com/acme/widgets/api/v2\n\ngo 1.22\n")
	u.change("feat(api)!: v2 module path")
	plan, err := u.planUnits()
	assert.NilError(t, err)
	assert.Equal(t, plan.Units[1].NextRelease.GitTag, "api/v2.0.0")
}

func TestPlanUnitsMajorCheck(t *testing.T) {
	cfg := unitsConfig + `  - - "@semantic-release/exec"
    - generateNotesCmd: ./scripts/shell-wrapper.sh major-release-checker.js ${lastRelease.version} ${nextRelease.version}
`
	u := baselined(t, cfg)
	u.change("feat(client)!: break", "node/widgets-client/index.js")
	_, err := u.planUnits()
	assert.ErrorIs(t, err, ErrMajorVersion)
	u.units[0].AllowMajorVersions = true
	plan, err := u.planUnits()
	assert.NilError(t, err)
	assert.Equal(t, versions(plan), "root=- widgets-client=2.0.0")
}

func TestPlanSchema2RoundTrip(t *testing.T) {
	u := baselined(t, unitsConfig)
	u.change("feat: both", "main.go", "node/widgets-client/index.js")
	plan, err := u.planUnits()
	assert.NilError(t, err)
	file := filepath.Join(t.TempDir(), "plan.json")
	assert.NilError(t, WritePlan(file, plan))
	got, err := ReadPlan(file, u.unitRepo())
	assert.NilError(t, err)
	assert.DeepEqual(t, got, plan)
	b, err := os.ReadFile(file)
	assert.NilError(t, err)
	assert.Assert(t, strings.Contains(string(b), `"schema": 2,`))
	assert.Assert(t, strings.Contains(string(b), `"units": [`))

	// Other units, or none, change the digest.
	repo, err := LoadRepo(u.git.Dir)
	assert.NilError(t, err)
	_, err = ReadPlan(file, repo)
	assert.ErrorIs(t, err, ErrPlanConfig)

	// A schema that publish does not know is refused.
	assert.NilError(t, os.WriteFile(file, []byte(strings.Replace(string(b), `"schema": 2`, `"schema": 3`, 1)), 0o600))
	_, err = ReadPlan(file, u.unitRepo())
	assert.ErrorIs(t, err, ErrPlanSchema)
}

func TestSetUnitsChecksFiles(t *testing.T) {
	u := newUnitFixture(t, unitsConfig)
	writeFile(t, u.git.Dir, "node/widgets-client/.releaserc.yaml", unitsConfig)
	repo, err := LoadRepo(u.git.Dir)
	assert.NilError(t, err)
	assert.ErrorIs(t, repo.SetUnits(u.units, u.root), ErrUnitConfig)
	assert.NilError(t, os.Remove(filepath.Join(u.git.Dir, "node", "widgets-client", ".releaserc.yaml")))
	repo, err = LoadRepo(u.git.Dir)
	assert.NilError(t, err)
	err = repo.SetUnits([]spec.Unit{nodeUnit("missing")}, spec.RootPaths{})
	assert.ErrorIs(t, err, errNPMNoPkg)
	err = repo.SetUnits([]spec.Unit{{Kind: spec.UnitGoModule, Dir: "nomod"}}, spec.RootPaths{})
	assert.ErrorIs(t, err, os.ErrNotExist)

	// A Go unit that pins the root go.mod needs a release commit with it.
	writeFile(t, u.git.Dir, "go.mod", "module github.com/acme/widgets\n\nrequire (\n\tgithub.com/acme/widgets/api v0.0.0 // pin\n)\n")
	writeFile(t, u.git.Dir, "api/go.mod", "module github.com/acme/widgets/api\n")
	err = repo.SetUnits([]spec.Unit{{Kind: spec.UnitGoModule, Dir: "api"}}, spec.RootPaths{})
	assert.ErrorContains(t, err, "needs a release commit with go.mod")
	pinNever := []spec.Unit{{Kind: spec.UnitGoModule, Dir: "api", Go: &spec.GoUnit{PinRoot: spec.GoPinNever}}}
	assert.NilError(t, repo.SetUnits(pinNever, spec.RootPaths{}))
	g := repo.Config.Units[0].Go
	assert.Assert(t, g.RootRequires && !g.RootReplaces)
	assert.Equal(t, repo.Config.Units[0].TagFormat, spec.TagFormat("api/v${version}"))
}
