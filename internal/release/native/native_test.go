// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Tests for the loader of the release section of
// scripts/devbase.yaml: the schema, the defaults and the errors.

package native_test

import (
	"strings"
	"testing"

	"github.com/getoutreach/devbase/v2/internal/release/native"
	"github.com/getoutreach/devbase/v2/internal/release/spec"
	"gotest.tools/v3/assert"
)

// minimal is the smallest valid section, with another tool's section.
const minimal = `graphql:
  lint:
    exclude: [vendor/**]
release:
  branches:
    - name: main
`

// fullSection is the example of the package documentation: a CLI repo
// with prereleases, with every key set.
const fullSection = `release:
  branches:
    - name: main
      prerelease: rc
    - name: release
  allowMajorVersions: false
  rules:
    - breaking: true
      release: minor
    - type: revert
      release: patch
    - type: perf
      release: patch
  build:
    before: [make dep]
    builds:
      - binary: widgets
        id: widgets
        main: ./cmd/widgets
        ldflags:
          - -w -s -X "github.com/getoutreach/gobox/pkg/app.Version=v{{ .Version }}"
          - -X "main.HoneycombTracingKey={{ .Env.HONEYCOMB_APIKEY }}"
          - -X "main.TeleforkAPIKey={{ .Env.TELEFORK_APIKEY }}"
        env: [CGO_ENABLED=0]
      - binary: widgetctl
    targets: [linux/amd64, linux/arm64, darwin/arm64]
    archive:
      files:
        - src: README*
        - src: release/yq
          dst: yq
          mode: 0755
    checksums: checksums.txt
  publishers:
    - notes
    - github:
        assets: [dist/*.tar.gz, dist/checksums.txt]
    - npm:
        packageRoot: api/clients/node
    - gem:
        dir: api/clients/ruby
        name: widgets_client
        versionFile: lib/widgets_client/version.rb
        owner: acme
    - python:
        dir: api/clients/python
        domain: acme
        account: "123456789012"
        region: eu-west-1
        repository: acme-pypi
    - hooks:
        generateNotes: ./scripts/notes.sh ${nextRelease.version}
        prepare: mise run orb:build
        publish: ./scripts/publish.sh ${nextRelease.version}
        success: echo done
    - commit:
        files: [CHANGELOG.md]
    - orb:
        name: getoutreach/shared
        path: orb.yml
`

// stencilLDFlags are the ldflags that stencil-golang renders.
var stencilLDFlags = []string{
	`-w -s -X "github.com/getoutreach/gobox/pkg/app.Version=v{{ .Version }}"`,
	`-X "main.HoneycombTracingKey={{ .Env.HONEYCOMB_APIKEY }}"`,
	`-X "main.TeleforkAPIKey={{ .Env.TELEFORK_APIKEY }}"`,
}

// defaultTargets are the targets that stencil-golang renders.
var defaultTargets = []spec.Target{
	spec.NewTarget("linux", "amd64"), spec.NewTarget("linux", "arm64"), spec.NewTarget("darwin", "arm64"),
}

// stencilBuild returns the build of binary that stencil-golang renders.
func stencilBuild(binary string) spec.Build {
	return spec.Build{
		ID: binary, Binary: binary, Main: "./cmd/" + binary, LDFlags: stencilLDFlags, Env: []string{"CGO_ENABLED=0"},
	}
}

// parse parses a section and fails the test on an error.
func parse(t *testing.T, s string) *spec.Config {
	t.Helper()
	cfg, warnings, err := native.Parse([]byte(s))
	assert.NilError(t, err)
	assert.Equal(t, len(warnings), 0)
	return cfg
}

func TestParseDefaults(t *testing.T) {
	assert.DeepEqual(t, parse(t, minimal), &spec.Config{
		Branches: []spec.Branch{{Name: "main"}},
		Rules: []spec.Rule{
			{Breaking: true, Release: spec.BumpMinor},
			{Type: "revert", Release: spec.BumpPatch},
			{Type: "perf", Release: spec.BumpPatch},
		},
		MajorCheck: true,
		Plugins: []spec.Plugin{
			{Notes: &spec.NotesPlugin{}},
			{Forge: &spec.ForgePlugin{Provider: spec.ForgeGitHub}},
		},
	})

	// With allowMajorVersions, the default rules have no breaking rule.
	cfg := parse(t, minimal+"  allowMajorVersions: true\n")
	assert.Assert(t, !cfg.MajorCheck)
	assert.DeepEqual(t, cfg.Rules, []spec.Rule{
		{Type: "revert", Release: spec.BumpPatch}, {Type: "perf", Release: spec.BumpPatch},
	})

	// A build has the stencil-golang values, and the GitHub release
	// uploads the archives and the checksum file.
	cfg = parse(t, minimal+"  build:\n    builds:\n      - binary: widgets\n")
	assert.Assert(t, cfg.Build)
	assert.DeepEqual(t, cfg.Artifacts, &spec.Artifacts{
		Hooks:   []string{"make dep"},
		Builds:  []spec.Build{stencilBuild("widgets")},
		Targets: defaultTargets,
		Files: []spec.ArchiveFile{
			{Src: "license*"}, {Src: "LICENSE*"}, {Src: "readme*"}, {Src: "README*"}, {Src: "changelog*"}, {Src: "CHANGELOG*"},
		},
		Checksums: "checksums.txt",
	})
	assert.DeepEqual(t, cfg.Forge().Assets, []string{"dist/*.tar.gz", "dist/checksums.txt"})
}

func TestParseFullSection(t *testing.T) {
	assert.DeepEqual(t, parse(t, fullSection), &spec.Config{
		Branches: []spec.Branch{{Name: "main", Prerelease: "rc", Channel: "main"}, {Name: "release"}},
		Rules: []spec.Rule{
			{Breaking: true, Release: spec.BumpMinor},
			{Type: "revert", Release: spec.BumpPatch},
			{Type: "perf", Release: spec.BumpPatch},
		},
		MajorCheck: true,
		Build:      true,
		Plugins: []spec.Plugin{
			{Notes: &spec.NotesPlugin{}},
			{Forge: &spec.ForgePlugin{Provider: spec.ForgeGitHub, Assets: []string{"dist/*.tar.gz", "dist/checksums.txt"}}},
			{NPM: &spec.NPMPlugin{PkgRoot: "api/clients/node"}},
			{Gem: &spec.GemPlugin{
				Dir: "api/clients/ruby", Name: "widgets_client", VersionFile: "lib/widgets_client/version.rb", Owner: "acme",
			}},
			{Python: &spec.PythonPlugin{
				Dir: "api/clients/python", Domain: "acme", Account: "123456789012", Region: "eu-west-1",
				Repository: "acme-pypi",
			}},
			{Exec: &spec.ExecHooks{
				GenerateNotes: "./scripts/notes.sh ${nextRelease.version}",
				Prepare:       "mise run orb:build",
				Publish:       "./scripts/publish.sh ${nextRelease.version}",
				Success:       "echo done",
			}},
			{Commit: &spec.CommitPlugin{Files: []string{"CHANGELOG.md"}}},
			{Orb: &spec.OrbPlugin{Name: "getoutreach/shared", Path: "orb.yml"}},
		},
		Artifacts: &spec.Artifacts{
			Hooks:   []string{"make dep"},
			Builds:  []spec.Build{stencilBuild("widgets"), stencilBuild("widgetctl")},
			Targets: defaultTargets,
			Files: []spec.ArchiveFile{
				{Src: "README*"}, {Src: "release/yq", Dst: "yq", Mode: 0o755},
			},
			Checksums: "checksums.txt",
		},
	})
}

func TestParseWithoutSection(t *testing.T) {
	for name, s := range map[string]string{
		"empty file":      "",
		"comment only":    "# nothing\n",
		"another section": "graphql:\n  lint: {}\n",
	} {
		cfg, warnings, err := native.Parse([]byte(s))
		assert.NilError(t, err, name)
		assert.Assert(t, cfg == nil, name)
		assert.Equal(t, len(warnings), 0, name)
	}
}

func TestParseEmptyLists(t *testing.T) {
	cfg := parse(t, minimal+`  rules: []
  build:
    before: []
    builds:
      - binary: tool
        main: .
        ldflags: [-s]
        env: []
    archive:
      files: []
  publishers:
    - github:
        assets: []
    - commit:
        files: []
    - commit
`)
	// An empty list replaces the default.
	assert.Assert(t, cfg.Rules == nil)
	a := cfg.Artifacts
	assert.Assert(t, a.Hooks == nil)
	assert.Assert(t, a.Builds[0].Env == nil)
	assert.Assert(t, a.Files == nil)
	assert.Assert(t, cfg.Plugins[0].Forge.Assets == nil)
	// An empty commit list commits nothing; no list commits the default
	// files.
	assert.DeepEqual(t, cfg.Plugins[1].Commit.Files, []string{})
	assert.Assert(t, cfg.Plugins[2].Commit.Files == nil)

	// No publishers is a release that only tags.
	cfg = parse(t, minimal+"  publishers: []\n")
	assert.Assert(t, cfg.Plugins == nil)
}

func TestParseWarnings(t *testing.T) {
	_, warnings, err := native.Parse([]byte(minimal + "  publishers:\n    - hooks:\n        publish: x ${options.dryrun}\n"))
	assert.NilError(t, err)
	assert.Equal(t, len(warnings), 1)

	// The release commit leaves out the client version files
	// (requirement 12), with a warning for each.
	cfg, warnings, err := native.Parse([]byte(minimal + `  publishers:
    - npm:
        packageRoot: api/clients/node
    - gem
    - python:
        domain: acme
        region: eu-west-1
        repository: acme-pypi
    - commit:
        files:
          - api/clients/node/package.json
          - api/clients/ruby/lib/widgets_client/version.rb
          - api/clients/python/pyproject.toml
          - go.mod
`))
	assert.NilError(t, err)
	assert.DeepEqual(t, cfg.Plugins[3].Commit.Files, []string{"go.mod"})
	assert.Equal(t, len(warnings), 3, strings.Join(warnings, "\n"))
}

// TestParseUnits checks that the section gives the root path rules and
// the units as it has them: the loader of the repository adds the
// defaults.
func TestParseUnits(t *testing.T) {
	cfg := parse(t, minimal+`  root:
    exclude: [node/**]
    shared: []
  units:
    - kind: npm
      dir: node/widgets-client
      paths: []
      exclude: [node/widgets-client/README.md]
      tagFormat: nodejs-widgets-client-v${version}
      skipPrereleases: true
      forgeRelease: true
      npm:
        install: yarn install --frozen-lockfile
    - name: api
      kind: go-module
      dir: api
      strategy: independent
      allowMajorVersions: true
      go:
        pinRoot: never
`)
	assert.DeepEqual(t, cfg.RootPaths, spec.RootPaths{Exclude: []string{"node/**"}})
	assert.DeepEqual(t, cfg.Units, []spec.Unit{
		{
			Kind: spec.UnitNPM, Dir: "node/widgets-client", Exclude: []string{"node/widgets-client/README.md"},
			TagFormat: "nodejs-widgets-client-v${version}", SkipPrereleases: true, ForgeRelease: true,
			NPM: &spec.NPMPlugin{Install: "yarn install --frozen-lockfile"},
		},
		{
			Name: "api", Kind: spec.UnitGoModule, Dir: "api", Strategy: spec.StrategyIndependent, AllowMajorVersions: true,
			Go: &spec.GoUnit{PinRoot: spec.GoPinNever},
		},
	})
}

// TestParseClientPublishers checks the defaults of the gem and python
// publishers, which are the folders that the stencil client hooks use.
func TestParseClientPublishers(t *testing.T) {
	cfg := parse(t, minimal+`  publishers:
    - gem
    - python:
        domain: acme
        region: eu-west-1
        repository: acme-pypi
`)
	assert.DeepEqual(t, cfg.Plugins, []spec.Plugin{
		{Gem: &spec.GemPlugin{Dir: spec.DefaultGemDir}},
		{Python: &spec.PythonPlugin{Dir: spec.DefaultPythonDir, Domain: "acme", Region: "eu-west-1", Repository: "acme-pypi"}},
	})
}

// TestParseRejectsUnknownKeys checks that a key that the section does not
// have is an error at its line, at every level.
func TestParseRejectsUnknownKeys(t *testing.T) {
	build := minimal + "  build:\n    builds:\n      - binary: widgets\n"
	for _, tc := range []struct{ name, section, wantErr string }{
		{"section", minimal + "  modules: []\n", "line 7: field modules not found"},
		{"root", minimal + "  root:\n    include: [a]\n", "line 8: field include not found"},
		{"unit", minimal + "  units:\n    - kind: generic\n      dir: a\n      tag: x\n", "line 10: field tag not found"},
		{"unit npm", minimal + "  units:\n    - kind: npm\n      dir: a\n      npm:\n        pkgRoot: a\n", "line 11: field pkgRoot not found"},
		{
			"unit go", minimal + "  units:\n    - kind: go-module\n      dir: a\n      go:\n        modulePath: x\n",
			"line 11: field modulePath not found",
		},
		{"branch", minimal + "      channel: next\n", "line 7: field channel not found"},
		{"rule", minimal + "  rules:\n    - type: feat\n      releas: minor\n", "line 9: field releas not found"},
		{"build", build + "    format: zip\n", "line 10: field format not found"},
		{"go build", build + "        goos: [linux]\n", "line 10: field goos not found"},
		{"archive", build + "    archive:\n      format: zip\n", "line 11: field format not found"},
		{"archive file", build + "    archive:\n      files:\n        - src: a\n          info: {}\n", "line 13: field info not found"},
		{"publisher", minimal + "  publishers:\n    - gitlab\n", `line 8: publisher "gitlab": unknown publisher`},
		{"publisher option", minimal + "  publishers:\n    - github:\n        asset: [a]\n", `line 9: unknown key "asset"`},
		{"hooks option", minimal + "  publishers:\n    - hooks:\n        prepareCmd: x\n", `line 9: unknown key "prepareCmd"`},
		{"gem option", minimal + "  publishers:\n    - gem:\n        version: x\n", `line 9: unknown key "version"`},
		{"python option", minimal + "  publishers:\n    - python:\n        package: x\n", `line 9: unknown key "package"`},
		{"two publishers in one entry", minimal + "  publishers:\n    - {notes: {}, github: {}}\n", "a publisher is a name"},
		{"options that are not a map", minimal + "  publishers:\n    - github: [a]\n", "the options are not a map"},
	} {
		_, _, err := native.Parse([]byte(tc.section))
		assert.ErrorIs(t, err, spec.ErrUnsupported, tc.name)
		assert.ErrorContains(t, err, tc.wantErr, tc.name)
		assert.ErrorContains(t, err, "scripts/devbase.yaml: release: ", tc.name)
	}
}

// acmePypi are the CodeArtifact domain and repository options of a
// python publisher.
const acmePypi = "        domain: acme\n        repository: acme-pypi\n"

// TestParseFailsClosed checks the errors of values that the release
// command cannot run.
func TestParseFailsClosed(t *testing.T) {
	build := minimal + "  build:\n    builds:\n      - binary: widgets\n"
	hooks := minimal + "  publishers:\n    - hooks:\n        "
	for _, tc := range []struct{ name, section, wantErr string }{
		{"empty section", "release:\n", "the section is empty"},
		{"no branches", "release:\n  allowMajorVersions: true\n", "no branches"},
		{"empty prerelease", minimal + "      prerelease: ''\n", "empty prerelease"},
		{"two release branches", minimal + "    - name: next\n", "2 release branches"},
		{"numeric prerelease", minimal + "    - name: next\n      prerelease: '1'\n", "never matches its tags"},
		{"maintenance branch", minimal + "    - name: 1.x\n      prerelease: beta\n", "maintenance branch"},
		{"rule release false", minimal + "  rules:\n    - type: chore\n      release: false\n", `rule release "false"`},
		{"rule without release", minimal + "  rules:\n    - type: chore\n", `rule release ""`},
		{"rule empty glob", minimal + "  rules:\n    - type: ''\n      release: patch\n", "empty type"},
		{"rule brace glob", minimal + "  rules:\n    - scope: '{a,b}'\n      release: patch\n", `scope "{a,b}"`},
		{"no builds", minimal + "  build:\n    builds: []\n", "no builds"},
		{"build without binary", minimal + "  build:\n    builds:\n      - main: .\n", "builds[0] without a binary"},
		{"duplicate binary", build + "      - binary: widgets\n        id: widgets2\n", `two builds with binary "widgets"`},
		{"target without arch", build + "    targets: [linux]\n", `target "linux" is not os/arch`},
		{"unsupported target", build + "    targets: [windows/mips]\n", "GoReleaser does not build"},
		{"ldflags template", build + "        ldflags: ['-X a={{ .Date }}']\n", ".Date"},
		{"archive outside repo", build + "    archive:\n      files:\n        - src: ../x\n", "outside the repository"},
		{"checksums path", build + "    checksums: dist/sums.txt\n", "checksum file name"},
		{"hooks without commands", minimal + "  publishers:\n    - hooks\n", "hooks without a command"},
		{"hooks with lookup", hooks + "publish: x ${nextRelease.notes}\n", "nextRelease.notes"},
		{"hooks with build", hooks + "generateNotes: make release APP_VERSION=${nextRelease.version}\n", "use the build key"},
		{"hooks with build task", hooks + "prepare: mise run release:build\n", "use the build key"},
		{"hooks with major check", hooks + "generateNotes: ./scripts/shell-wrapper.sh major-release-checker.js\n", "allowMajorVersions"},
		{"orb without name", minimal + "  publishers:\n    - orb\n", "orb plugin without a name"},
		{"hooks with Ruby build", hooks + "generateNotes: ./scripts/shell-wrapper.sh ruby/build.sh 1.0.0\n", "use the gem or python publisher"},
		{"hooks with Ruby publish", hooks + "publish: ./scripts/shell-wrapper.sh ruby/publish.sh 1.0.0\n", "use the gem or python publisher"},
		{"hooks with poetry", hooks + "generateNotes: cd py && poetry version ${nextRelease.version}\n", "use the gem or python publisher"},
		{"hooks with pyclient", hooks + "publish: mise run publish:pyclient\n", "use the gem or python publisher"},
		{"hooks with submodules", hooks + "prepare: ./scripts/release-submodules.sh ${nextRelease.version}\n", "as units"},
		{"gem name", minimal + "  publishers:\n    - gem:\n        name: bad name\n", `gem name "bad name"`},
		{"gem dir", minimal + "  publishers:\n    - gem:\n        dir: ../ruby\n", "not a clean relative path"},
		{"python region", minimal + "  publishers:\n    - python:\n        region: west\n" + acmePypi, `AWS region "west"`},
		{"python without repository", minimal + "  publishers:\n    - python\n", "needs the CodeArtifact domain, repository and region"},
		{
			"python prerelease", minimal + "    - name: next\n      prerelease: next\n  publishers:\n    - python:\n" +
				acmePypi + "        region: eu-west-1\n", "PEP 440",
		},
		{"github glob", minimal + "  publishers:\n    - github:\n        assets: ['dist/**']\n", "file glob"},
	} {
		_, _, err := native.Parse([]byte(tc.section))
		assert.ErrorIs(t, err, spec.ErrUnsupported, tc.name)
		assert.ErrorContains(t, err, tc.wantErr, tc.name)
	}

	_, _, err := native.Parse([]byte("release: [\n"))
	assert.Assert(t, err != nil)
	assert.Assert(t, strings.HasPrefix(err.Error(), "parse scripts/devbase.yaml"), err.Error())
}
