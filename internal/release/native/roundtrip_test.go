// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Tests that a converted configuration loads as the same
// release model, for the surveyed .releaserc.yaml and .goreleaser.yml
// files.

package native_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/getoutreach/devbase/v2/internal/release/goreleaser"
	"github.com/getoutreach/devbase/v2/internal/release/native"
	"github.com/getoutreach/devbase/v2/internal/release/releaserc"
	"github.com/getoutreach/devbase/v2/internal/release/spec"
	"gotest.tools/v3/assert"
)

// roundTrip checks that cfg, written by Marshal and loaded by Parse, is
// cfg, and returns the section.
func roundTrip(t *testing.T, cfg *spec.Config) string {
	t.Helper()
	b, err := native.Marshal(cfg)
	assert.NilError(t, err)
	got, _, err := native.Parse(b)
	assert.NilError(t, err)
	assert.DeepEqual(t, got, cfg)
	return string(b)
}

// readFile returns the contents of a test file.
func readFile(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	assert.NilError(t, err)
	return b
}

// TestRoundTrip converts the 15 surveyed .releaserc.yaml files, the edge
// case file and the Python client file. A config with the build hook is
// converted with each .goreleaser.yml fixture.
func TestRoundTrip(t *testing.T) {
	setCodeArtifactEnv(t)
	rcs, err := filepath.Glob("testdata/survey/*.releaserc.yaml")
	assert.NilError(t, err)
	assert.Equal(t, len(rcs), 15)
	rcs = append(rcs, "testdata/edge.releaserc.yaml", "testdata/python-client.releaserc.yaml")
	grs, err := filepath.Glob("testdata/goreleaser/*.goreleaser.yml")
	assert.NilError(t, err)
	assert.Assert(t, len(grs) > 0)

	for _, rc := range rcs {
		t.Run(filepath.Base(rc), func(t *testing.T) {
			cfg, _, err := releaserc.Parse(readFile(t, rc))
			assert.NilError(t, err)
			if !cfg.Build {
				roundTrip(t, cfg)
				return
			}
			for _, gr := range grs {
				t.Run(filepath.Base(gr), func(t *testing.T) {
					a, err := goreleaser.Parse(readFile(t, gr))
					assert.NilError(t, err)
					c := *cfg
					c.Artifacts = a
					roundTrip(t, &c)
				})
			}
		})
	}
}

// TestMarshalDevbase checks the section of the devbase configuration: it
// has only the values that are not the defaults.
func TestMarshalDevbase(t *testing.T) {
	cfg, _, err := releaserc.Parse(readFile(t, "testdata/survey/devbase.releaserc.yaml"))
	assert.NilError(t, err)
	cfg.Artifacts, err = goreleaser.Parse(readFile(t, "testdata/goreleaser/devbase.goreleaser.yml"))
	assert.NilError(t, err)
	assert.Equal(t, roundTrip(t, cfg), `release:
  branches:
    - name: main
      prerelease: rc
    - name: release
  allowMajorVersions: true
  build:
    builds:
      - binary: devbase
    archive:
      files:
        - src: license*
        - src: LICENSE*
        - src: readme*
        - src: README*
        - src: release/yq
          dst: yq
          mode: 0755
        - src: release/tomlq
          dst: tomlq
          mode: 0755
  publishers:
    - notes
    - github
    - hooks:
        prepare: mise run orb:build
    - orb:
        name: getoutreach/shared
        path: orb.yml
`)
}

// setCodeArtifactEnv sets the CodeArtifact repository of the Python
// client hooks.
func setCodeArtifactEnv(t *testing.T) {
	t.Helper()
	t.Setenv(releaserc.CodeArtifactDomainEnv, "acme")
	t.Setenv(releaserc.CodeArtifactRepositoryEnv, "acme-pypi")
	t.Setenv(releaserc.CodeArtifactRegionEnv, "eu-west-1")
}

// TestMarshalClients checks the sections of the configs with client
// publishers: the gem and python publishers have no default values, the
// python publisher has the CodeArtifact repository of the environment,
// and the release commit has no client version files.
func TestMarshalClients(t *testing.T) {
	setCodeArtifactEnv(t)
	for rc, want := range map[string]string{
		"testdata/survey/service-npm-gem.releaserc.yaml": `release:
  branches:
    - name: main
  rules:
    - type: revert
      release: patch
    - type: perf
      release: patch
  publishers:
    - notes
    - npm:
        packageRoot: api/clients/node
    - gem
    - commit:
        files: []
    - github
`,
		"testdata/python-client.releaserc.yaml": `release:
  branches:
    - name: main
  rules:
    - type: revert
      release: patch
    - type: perf
      release: patch
  publishers:
    - notes
    - commit:
        files: []
    - python:
        domain: acme
        region: eu-west-1
        repository: acme-pypi
    - github
`,
	} {
		cfg, warnings, err := releaserc.Parse(readFile(t, rc))
		assert.NilError(t, err)
		assert.Assert(t, len(warnings) > 0, rc)
		assert.Equal(t, roundTrip(t, cfg), want, rc)
	}

	// Values that are not the defaults stay.
	cfg, _, err := native.Parse([]byte(fullSection))
	assert.NilError(t, err)
	cfg.Plugins = []spec.Plugin{
		{Gem: &spec.GemPlugin{Name: "widgets_client"}},
		{Python: &spec.PythonPlugin{Dir: "py", Domain: "acme", Region: "eu-west-1", Repository: "acme-pypi"}},
	}
	b, err := native.Marshal(cfg)
	assert.NilError(t, err)
	assert.Assert(t, strings.HasSuffix(string(b), `  publishers:
    - gem:
        name: widgets_client
    - python:
        dir: py
        domain: acme
        region: eu-west-1
        repository: acme-pypi
`), string(b))
}

func TestMarshalOmitsDefaults(t *testing.T) {
	for _, s := range []string{
		"release:\n  branches:\n    - name: main\n",
		"release:\n  branches:\n    - name: main\n  build:\n    builds:\n      - binary: widgets\n",
	} {
		cfg, _, err := native.Parse([]byte(s))
		assert.NilError(t, err)
		assert.Equal(t, roundTrip(t, cfg), s)
	}
	cfg, _, err := native.Parse([]byte(fullSection))
	assert.NilError(t, err)
	roundTrip(t, cfg)
}

func TestMarshalRejectsWhatTheSectionCannotHold(t *testing.T) {
	cfg, _, err := native.Parse([]byte(fullSection))
	assert.NilError(t, err)
	for name, change := range map[string]func(c *spec.Config){
		"release branch with a channel": func(c *spec.Config) { c.Branches[1].Channel = "stable" },
		"prerelease channel":            func(c *spec.Config) { c.Branches[0].Channel = "beta" },
		"build without artifacts":       func(c *spec.Config) { c.Artifacts = nil },
		"artifacts without a build":     func(c *spec.Config) { c.Build = false },
		"invalid config":                func(c *spec.Config) { c.Branches = nil },
	} {
		c := *cfg
		c.Branches = append([]spec.Branch{}, cfg.Branches...)
		change(&c)
		_, err := native.Marshal(&c)
		assert.ErrorIs(t, err, spec.ErrUnsupported, name)
	}
}
