// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Tests for the Python client hooks of the loader.

package releaserc

import (
	"strings"
	"testing"

	"github.com/getoutreach/devbase/v2/internal/release/spec"
	"gotest.tools/v3/assert"
)

// rubyPythonConfig is a service config with Ruby and Python clients, and
// a release commit of their version files that a person added.
const rubyPythonConfig = `preset: conventionalcommits
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
  - - "@semantic-release/exec"
    - generateNotesCmd: |-
        ./scripts/shell-wrapper.sh ruby/build.sh ${nextRelease.version} 1>&2
      publishCmd: |-
        DRYRUN=${options.dryrun} ./scripts/shell-wrapper.sh ruby/publish.sh ${nextRelease.version}
  - - "@semantic-release/git"
    - assets:
        - api/clients/ruby/lib/widgets_client/version.rb
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
  - "@semantic-release/github"
`

// allClientsConfig is a CLI service config with Node.js, Ruby and Python
// clients, and a release commit for go.mod and go.sum.
const allClientsConfig = `preset: conventionalcommits
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
  - - "@semantic-release/exec"
    - generateNotesCmd: "make release APP_VERSION=${nextRelease.version}"
  - "@semantic-release/release-notes-generator"
  - - "@semantic-release/npm"
    - pkgRoot: api/clients/node
  - - "@semantic-release/exec"
    - generateNotesCmd: |-
        ./scripts/shell-wrapper.sh ruby/build.sh ${nextRelease.version} 1>&2
      publishCmd: |-
        DRYRUN=${options.dryrun} ./scripts/shell-wrapper.sh ruby/publish.sh ${nextRelease.version}
  - - "@semantic-release/exec"
    - prepareCmd: "./scripts/release-submodules.sh ${nextRelease.version}"
  - - "@semantic-release/git"
    - assets:
        - api/clients/node/package.json
        - api/clients/ruby/lib/widgets_client/version.rb
        - go.mod
        - go.sum
  - - "@semantic-release/exec"
    - generateNotesCmd: |-
        cd api/clients/python && $(mise which poetry) version ${nextRelease.version}
      publishCmd: |-
        if [ "${options.dryrun}" = "true" ]; then
          mise run publish:pyclient --environment=production --skip-publish=true
        else
          mise run publish:pyclient --environment=production --skip-publish=false
        fi
  - - "@semantic-release/github"
    - assets:
        - "dist/*.tar.gz"
        - "dist/checksums.txt"
`

// pythonHooks is the stencil Python client exec entry with the given
// generateNotesCmd and publishCmd.
func pythonHooks(generateNotes, publish string) string {
	indent := func(s string) string { return strings.ReplaceAll(s, "\n", "\n        ") }
	return "preset: conventionalcommits\nbranches: [main]\nplugins:\n" +
		"  - - \"@semantic-release/exec\"\n" +
		"    - generateNotesCmd: |-\n        " + indent(generateNotes) + "\n" +
		"      publishCmd: |-\n        " + indent(publish) + "\n"
}

// setCodeArtifactEnv sets the CodeArtifact repository of the Python
// client hooks.
func setCodeArtifactEnv(t *testing.T) {
	t.Helper()
	t.Setenv(CodeArtifactDomainEnv, "acme")
	t.Setenv(CodeArtifactRepositoryEnv, "acme-pypi")
	t.Setenv(CodeArtifactRegionEnv, "eu-west-1")
}

func TestParsePythonHooks(t *testing.T) {
	setCodeArtifactEnv(t)
	cfg, warnings, err := Parse([]byte(rubyPythonConfig))
	assert.NilError(t, err)
	assert.DeepEqual(t, cfg.Plugins, []spec.Plugin{
		{Notes: &spec.NotesPlugin{}},
		{Gem: &spec.GemPlugin{Dir: "api/clients/ruby"}},
		// Both assets are client version files.
		{Commit: &spec.CommitPlugin{Files: []string{}}},
		{Python: &spec.PythonPlugin{
			Dir: "api/clients/python", Domain: "acme", Region: "eu-west-1", Repository: "acme-pypi",
		}},
		{Forge: &spec.ForgePlugin{Provider: spec.ForgeGitHub}},
	})
	assert.Assert(t, cfg.MajorCheck)
	assert.DeepEqual(t, warnings, []string{
		`the release does not commit the client version file "api/clients/ruby/lib/widgets_client/version.rb"; ` +
			"remove it from the release commit files",
		`the release does not commit the client version file "api/clients/python/pyproject.toml"; ` +
			"remove it from the release commit files",
	})

	// The npm and Ruby version files leave the release commit, which
	// keeps go.mod and go.sum.
	cfg, _, err = Parse([]byte(allClientsConfig))
	assert.NilError(t, err)
	assert.DeepEqual(t, cfg.Plugins[4].Commit, &spec.CommitPlugin{Files: []string{"go.mod", "go.sum"}})
	assert.Assert(t, cfg.Plugins[5].Python != nil)

	// The hooks alone are the Python plugin.
	cfg, _, err = Parse([]byte(pythonHooks(pythonVersionCmd, pythonPublishCmd)))
	assert.NilError(t, err)
	assert.Equal(t, len(cfg.Plugins), 1)
	assert.Assert(t, cfg.Plugins[0].Python != nil)
}

// TestParsePythonHooksNeedTheRepository checks that the hooks fail
// closed without the CodeArtifact repository, and that the error names
// the environment variables and the release section.
func TestParsePythonHooksNeedTheRepository(t *testing.T) {
	for _, env := range []string{CodeArtifactDomainEnv, CodeArtifactRepositoryEnv, CodeArtifactRegionEnv} {
		setCodeArtifactEnv(t)
		t.Setenv(env, "")
		_, _, err := Parse([]byte(pythonHooks(pythonVersionCmd, pythonPublishCmd)))
		assert.ErrorIs(t, err, spec.ErrUnsupported, env)
		for _, want := range []string{
			CodeArtifactDomainEnv, CodeArtifactRepositoryEnv, CodeArtifactRegionEnv,
			"python publisher of the release section of scripts/devbase.yaml",
		} {
			assert.ErrorContains(t, err, want, env)
		}
	}
}

func TestParsePythonHooksFailClosed(t *testing.T) {
	setCodeArtifactEnv(t)
	devPublish := strings.ReplaceAll(pythonPublishCmd, "production", "development")
	tests := map[string]string{
		"generateNotes hook only": "preset: conventionalcommits\nbranches: [main]\nplugins:\n" +
			"  - - \"@semantic-release/exec\"\n    - generateNotesCmd: '" + pythonVersionCmd + "'\n",
		"publish hook only": "preset: conventionalcommits\nbranches: [main]\nplugins:\n" +
			"  - - \"@semantic-release/exec\"\n    - publishCmd: 'mise run publish:pyclient --environment=production'\n",
		"other environment": pythonHooks(pythonVersionCmd, devPublish),
		"other package dir": pythonHooks(strings.Replace(pythonVersionCmd, "api/clients/python", "python", 1), pythonPublishCmd),
		"poetry from PATH":  pythonHooks("cd api/clients/python && poetry version ${nextRelease.version}", pythonPublishCmd),
		"poetry in prepare": "preset: conventionalcommits\nbranches: [main]\nplugins:\n" +
			"  - - \"@semantic-release/exec\"\n    - prepareCmd: 'cd py && poetry version ${nextRelease.version}'\n",
		"extra success hook": pythonHooks(pythonVersionCmd, pythonPublishCmd) + "      successCmd: echo done\n",
		"prerelease without a PEP 440 form": strings.Replace(pythonHooks(pythonVersionCmd, pythonPublishCmd),
			"branches: [main]", "branches: [release, {name: main, prerelease: next}]", 1),
	}
	for name, cfg := range tests {
		t.Run(name, func(t *testing.T) {
			_, _, err := Parse([]byte(cfg))
			assert.ErrorIs(t, err, spec.ErrUnsupported)
		})
	}
}
