// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Tests for the Python plugin and the PEP 440 versions.

package spec_test

import (
	"testing"

	"github.com/getoutreach/devbase/v2/internal/release/spec"
	"gotest.tools/v3/assert"
)

// pythonConfig returns a valid config with an rc branch and a Python
// plugin.
func pythonConfig(py *spec.PythonPlugin) *spec.Config {
	return &spec.Config{
		Branches: []spec.Branch{{Name: "main", Prerelease: "rc", Channel: "main"}, {Name: "release"}},
		Plugins:  []spec.Plugin{{Python: py}},
	}
}

// acmePython is a valid Python plugin with the default folder.
func acmePython() spec.PythonPlugin {
	return spec.PythonPlugin{Domain: "acme", Region: "eu-west-1", Repository: "acme-pypi"}
}

func TestPythonPluginDefaults(t *testing.T) {
	base := acmePython()
	want := acmePython()
	want.Dir = "api/clients/python"
	assert.DeepEqual(t, base.WithDefaults(), want)
	custom := spec.PythonPlugin{Dir: "py", Domain: "d1", Account: "123456789012", Region: "eu-west-1", Repository: "r1"}
	assert.DeepEqual(t, custom.WithDefaults(), custom)

	_, err := pythonConfig(&base).Validate()
	assert.NilError(t, err)
	_, err = pythonConfig(&custom).Validate()
	assert.NilError(t, err)
	assert.Assert(t, pythonConfig(&custom).IsClientVersionFile("py/pyproject.toml"))
	assert.Assert(t, pythonConfig(&base).IsClientVersionFile("api/clients/python/pyproject.toml"))
}

func TestPythonPluginFailsClosed(t *testing.T) {
	for name, change := range map[string]func(py *spec.PythonPlugin){
		"absolute dir":    func(py *spec.PythonPlugin) { py.Dir = "/api" },
		"dir outside":     func(py *spec.PythonPlugin) { py.Dir = "../api" },
		"unclean dir":     func(py *spec.PythonPlugin) { py.Dir = "api/./python" },
		"domain":          func(py *spec.PythonPlugin) { py.Domain = "Acme" },
		"repository":      func(py *spec.PythonPlugin) { py.Repository = "-pypi" },
		"region":          func(py *spec.PythonPlugin) { py.Region = "eu west 1" },
		"short account":   func(py *spec.PythonPlugin) { py.Account = "12345" },
		"account letters": func(py *spec.PythonPlugin) { py.Account = "12345678901a" },
		"no domain":       func(py *spec.PythonPlugin) { py.Domain = "" },
		"no repository":   func(py *spec.PythonPlugin) { py.Repository = "" },
		"no region":       func(py *spec.PythonPlugin) { py.Region = "" },
	} {
		t.Run(name, func(t *testing.T) {
			py := acmePython()
			change(&py)
			_, err := pythonConfig(&py).Validate()
			assert.ErrorIs(t, err, spec.ErrUnsupported)
		})
	}

	// A prerelease branch needs a PEP 440 form.
	py := acmePython()
	cfg := pythonConfig(&py)
	cfg.Branches[0].Prerelease = "next"
	_, err := cfg.Validate()
	assert.ErrorIs(t, err, spec.ErrUnsupported)
	assert.ErrorContains(t, err, `branch "main"`)
}

// TestPEP440Version checks the versions that poetry 2.4.3 writes for
// "poetry version <version>".
func TestPEP440Version(t *testing.T) {
	for version, want := range map[string]string{
		"1.2.0":           "1.2.0",
		"1.2.0-rc.1":      "1.2.0rc1",
		"1.2.0-rc.12":     "1.2.0rc12",
		"1.2.0-RC.1":      "1.2.0rc1",
		"1.2.0-alpha.3":   "1.2.0a3",
		"1.2.0-a.1":       "1.2.0a1",
		"1.2.0-beta.1":    "1.2.0b1",
		"1.2.0-b.2":       "1.2.0b2",
		"1.2.0-c.1":       "1.2.0rc1",
		"1.2.0-pre.1":     "1.2.0rc1",
		"1.2.0-preview.1": "1.2.0rc1",
		"1.2.0-dev.2":     "1.2.0.dev2",
		"0.1.0-rc.0":      "0.1.0rc0",
	} {
		got, err := spec.PEP440Version(version)
		assert.NilError(t, err, version)
		assert.Equal(t, got, want, version)
	}
	for _, version := range []string{
		"1.2.0-post.1",           // a post-release sorts after 1.2.0
		"1.2.0-foo.1",            // no PEP 440 identifier
		"1.2.0-rc",               // no number
		"1.2.0-rc.1.2",           // two numbers
		"1.2.0-rc.x",             // not a number
		"0.0.0-unstable+abc1234", // build metadata
		"1.2",                    // not semver
	} {
		_, err := spec.PEP440Version(version)
		assert.ErrorIs(t, err, spec.ErrUnsupported, version)
	}
}
