// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Tests for the client version files of requirement 12:
// a repository with clients makes no release commit for their versions.

package release

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

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
