// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Tests for the units that config convert finds in the old
// configuration.

package release

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/getoutreach/devbase/v2/internal/release/native"
	"github.com/getoutreach/devbase/v2/internal/release/releaserc"
	"github.com/getoutreach/devbase/v2/internal/release/spec"
	"gotest.tools/v3/assert"
)

// submodulesConfig is a .releaserc.yaml with a release-submodules.sh
// hook, which the placeholder HOOK sets, and the plugins that PLUGINS
// adds.
const submodulesConfig = `preset: conventionalcommits
branches:
  - name: main
plugins:
  - "@semantic-release/commit-analyzer"
  - "@semantic-release/release-notes-generator"
  - - "@semantic-release/exec"
    - HOOK
PLUGINS  - "@semantic-release/github"
`

// convertMigrate converts the old files in dir, checks the notes,
// replaces the old files with the section, and returns the loaded repo.
func convertMigrate(t *testing.T, dir string, wantNotes []string, remove ...string) (string, *Repo) {
	t.Helper()
	section, notes, err := ConvertConfig(dir)
	assert.NilError(t, err)
	assert.DeepEqual(t, notes, wantNotes)
	for _, name := range append(remove, releaserc.File) {
		assert.NilError(t, os.Remove(filepath.Join(dir, filepath.FromSlash(name))))
	}
	writeFiles(t, dir, map[string]string{native.File: string(section)})
	repo, err := LoadRepo(dir)
	assert.NilError(t, err)
	return string(section), repo
}

// TestConvertConfigMirrorSubmodule converts a hook whose script tags the
// api module with the root version. The other folders with a go.mod or
// a package.json are only in the notes.
func TestConvertConfigMirrorSubmodule(t *testing.T) {
	dir := t.TempDir()
	cfg := strings.NewReplacer(
		"HOOK", `prepareCmd: "./scripts/release-submodules.sh ${nextRelease.version}"
      successCmd: "echo done"`,
		"PLUGINS", `  - - "@semantic-release/git"
    - assets: [go.mod, go.sum]
  - - "@semantic-release/npm"
    - pkgRoot: api/clients/node
`).Replace(submodulesConfig)
	writeFiles(t, dir, map[string]string{
		releaserc.File:                     cfg,
		"scripts/release-submodules.sh":    "#!/bin/sh\nfor mod in api; do git tag \"$mod/v$1\"; done\n",
		"go.mod":                           "module github.com/acme/widgets\n\nrequire github.com/acme/widgets/api v1.0.0\n",
		"package.json":                     "{}\n",
		"api/go.mod":                       "module github.com/acme/widgets/api\n",
		"api/clients/node/package.json":    "{}\n",
		"tools/lint/go.mod":                "module github.com/acme/widgets/tools/lint\n",
		"node/widgets-client/package.json": "{}\n",
		"node/widgets-client/node_modules/x/package.json": "{}\n",
		".cache/x/go.mod":            "module x\n",
		"internal/testdata/x/go.mod": "module x\n",
	})
	section, repo := convertMigrate(t, dir, []string{
		"delete scripts/release-submodules.sh: the units replace it",
		"the Go modules in tools/lint are not units, because the old configuration does not release them; " +
			"add go-module units to release them",
		"the npm packages in node/widgets-client are not units, because the old configuration does not release them; " +
			"if other jobs release them, for example on a tag, add npm units",
	}, "scripts/release-submodules.sh")
	assert.Assert(t, strings.HasSuffix(section, `    - hooks:
        success: echo done
    - commit:
        files:
          - go.mod
          - go.sum
    - npm:
        packageRoot: api/clients/node
    - github
  units:
    - kind: go-module
      dir: api
`), section)
	assert.DeepEqual(t, repo.Config.Units, []spec.Unit{{
		Name: "api", Kind: spec.UnitGoModule, Dir: "api", Paths: []string{"api"}, TagFormat: "api/v${version}",
		Strategy: spec.StrategyMirror,
		Go:       &spec.GoUnit{ModulePath: "github.com/acme/widgets/api", PinRoot: spec.GoPinAuto, RootRequires: true},
	}})
}

// TestConvertConfigIndependentSubmodule converts a hook whose script
// releases a module with a nested semantic-release run. The old release
// does not commit go.mod, so the unit does not pin the root go.mod.
func TestConvertConfigIndependentSubmodule(t *testing.T) {
	dir := t.TempDir()
	cfg := strings.NewReplacer("HOOK", `successCmd: ./scripts/release-submodules.sh`, "PLUGINS", "").Replace(submodulesConfig)
	writeFiles(t, dir, map[string]string{
		releaserc.File:                  cfg,
		"scripts/release-submodules.sh": "#!/bin/sh\nmodules=(pkg/scope)\n",
		"go.mod":                        "module github.com/acme/widgets\n\nrequire github.com/acme/widgets/pkg/scope v1.0.0\n",
		"pkg/scope/go.mod":              "module github.com/acme/widgets/pkg/scope\n",
		"pkg/scope/.releaserc.yaml":     "tagFormat: pkg/scope/v${version}\nbranches: [main]\n",
	})
	section, repo := convertMigrate(t, dir, []string{
		"delete pkg/scope/.releaserc.yaml: the unit pkg/scope replaces it",
		"delete scripts/release-submodules.sh: the units replace it",
	}, "scripts/release-submodules.sh", "pkg/scope/.releaserc.yaml")
	assert.Assert(t, strings.HasSuffix(section, `  units:
    - kind: go-module
      dir: pkg/scope
      strategy: independent
      go:
        pinRoot: never
`), section)
	assert.Equal(t, repo.Config.Units[0].TagFormat, spec.TagFormat("pkg/scope/v${version}"))
}

// TestConvertConfigSubmodulesFailsClosed checks the hooks that the
// conversion cannot replace with units.
func TestConvertConfigSubmodulesFailsClosed(t *testing.T) {
	for name, tc := range map[string]struct{ hook, script string }{
		"script names no module": {"prepareCmd: ./scripts/release-submodules.sh", "#!/bin/sh\ngit tag openapi/v1\n"},
		"hook with other commands": {
			`prepareCmd: "./scripts/release-submodules.sh ${nextRelease.version} && echo done"`, "#!/bin/sh\necho api\n",
		},
	} {
		dir := t.TempDir()
		writeFiles(t, dir, map[string]string{
			releaserc.File:                  strings.NewReplacer("HOOK", tc.hook, "PLUGINS", "").Replace(submodulesConfig),
			"scripts/release-submodules.sh": tc.script,
			"api/go.mod":                    "module github.com/acme/widgets/api\n",
		})
		_, _, err := ConvertConfig(dir)
		assert.ErrorIs(t, err, spec.ErrUnsupported, name)
	}
}
