// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Tests for the npm registry, with cases ported from
// @semantic-release/npm 13.2.0 (test/*.test.js), and a fake runner.

package release

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/getoutreach/devbase/v2/internal/release/forge"
	"github.com/getoutreach/devbase/v2/internal/release/spec"
	"gotest.tools/v3/assert"
)

// errExit stands for the exit error of a failed command.
var errExit = errors.New("exit status 1")

// npmE404 is the stderr of npm 10 for a missing version.
const npmE404 = "npm error code E404\nnpm error 404 No match found for version 1.0.0\n"

// fakeReply is the scripted result of a command.
type fakeReply struct {
	stdout, stderr string
	err            error
}

// fakeCall is a command that the fake runner got, with the contents of
// its --userconfig file at that time.
type fakeCall struct {
	dir   string
	env   []string
	argv  []string
	npmrc string
}

// fakeRunner records the commands and returns scripted results by the
// command name and its first arguments, such as "npm view" or
// "circleci orb info".
type fakeRunner struct {
	calls   []fakeCall
	replies map[string]fakeReply
}

// run is the cmdRunner of the fake. It writes the masked command line to
// log, as execCommand does.
func (f *fakeRunner) run(ctx context.Context, log io.Writer, c *command) (cmdResult, error) {
	call := fakeCall{dir: c.dir, env: c.env, argv: append([]string{c.name}, c.args...)}
	if i := slices.Index(c.args, "--userconfig"); i >= 0 {
		b, _ := os.ReadFile(c.args[i+1])
		call.npmrc = string(b)
		call.argv[i+2] = "<npmrc>"
	}
	f.calls = append(f.calls, call)
	io.WriteString(log, maskerFor(ctx, c.env).Mask("Run "+c.String()+"\n"))
	key := c.name + " " + c.args[0]
	if c.args[0] == "orb" {
		key += " " + c.args[1]
	}
	r := f.replies[key]
	return cmdResult{stdout: r.stdout, stderr: r.stderr}, r.err
}

// argvs returns the argv of each call.
func (f *fakeRunner) argvs() [][]string {
	if len(f.calls) == 0 {
		return nil
	}
	out := make([][]string, 0, len(f.calls))
	for _, c := range f.calls {
		out = append(out, c.argv)
	}
	return out
}

// npmFixture is a repository with a package.json and an isolated HOME.
type npmFixture struct {
	t    *testing.T
	repo string
	home string
	env  []string
	run  *fakeRunner
	log  bytes.Buffer
}

// newNPMFixture makes a repository with pkgJSON at pkgRoot, and an env
// with HOME and the extra variables.
func newNPMFixture(t *testing.T, pkgRoot, pkgJSON string, extraEnv ...string) *npmFixture {
	t.Helper()
	f := &npmFixture{t: t, repo: t.TempDir(), home: t.TempDir(), run: &fakeRunner{replies: map[string]fakeReply{}}}
	if pkgJSON != "" {
		writeFile(t, f.repo, filepath.Join(pkgRoot, "package.json"), pkgJSON)
	}
	f.env = append([]string{"HOME=" + f.home, "PATH=/usr/bin"}, extraEnv...)
	return f
}

// publisher returns the registry target of version on channel.
func (f *npmFixture) publisher(version, channel string) *regTarget {
	return &regTarget{dir: f.repo, version: version, channel: channel, env: f.env, log: &f.log}
}

// registry returns the npm registry of pkgRoot with the fake runner.
func (f *npmFixture) registry(pkgRoot string) *npmRegistry {
	return &npmRegistry{plugin: &spec.NPMPlugin{PkgRoot: pkgRoot}, run: f.run.run}
}

// githubNpmrc is the ~/.npmrc of Outreach CI: the scope registry and a
// token from an environment variable.
const githubNpmrc = "@getoutreach:registry=https://npm.pkg.github.com\n//npm.pkg.github.com/:_authToken=${GH_TOKEN}\n"

// TestNPMPublishScenarios checks the argv, dir and env of the npm
// commands in prepare and publish.

func TestNPMPublishScenarios(t *testing.T) {
	const pkgRoot = "api/clients/node"
	const scoped = `{"name": "@getoutreach/client", "version": "0.0.0-development"}`
	tests := []struct {
		name     string
		pkg      string
		npmrc    string
		env      []string
		version  string
		channel  string
		replies  map[string]fakeReply
		wantArgv [][]string
		wantInfo *forge.Published
		wantErr  error
	}{
		{
			// Scenario 10 of the RFC: npm version, then npm publish with
			// dist-tag latest, to the scope registry in ~/.npmrc.
			name: "first publish", pkg: scoped, npmrc: githubNpmrc, version: "1.0.0",
			replies: map[string]fakeReply{"npm view": {stderr: npmE404, err: errExit}},
			wantArgv: [][]string{
				{"npm", "version", "1.0.0", "--userconfig", "<npmrc>", "--no-git-tag-version", "--allow-same-version"},
				{"npm", "view", "@getoutreach/client@1.0.0", "version", "--userconfig", "<npmrc>", "--registry", "https://npm.pkg.github.com/"},
				{"npm", "publish", "<pkg>", "--userconfig", "<npmrc>", "--tag", "latest", "--registry", "https://npm.pkg.github.com/"},
			},
			wantInfo: &forge.Published{Name: "npm package (@latest dist-tag)"},
		},
		{
			name: "first publish with old npm", pkg: scoped, npmrc: githubNpmrc, version: "1.0.0",
			replies: map[string]fakeReply{"npm view": {}},
			wantArgv: [][]string{
				{"npm", "version", "1.0.0", "--userconfig", "<npmrc>", "--no-git-tag-version", "--allow-same-version"},
				{"npm", "view", "@getoutreach/client@1.0.0", "version", "--userconfig", "<npmrc>", "--registry", "https://npm.pkg.github.com/"},
				{"npm", "publish", "<pkg>", "--userconfig", "<npmrc>", "--tag", "latest", "--registry", "https://npm.pkg.github.com/"},
			},
			wantInfo: &forge.Published{Name: "npm package (@latest dist-tag)"},
		},
		{
			name: "prerelease channel dist-tag", pkg: scoped, npmrc: githubNpmrc, version: "1.0.0-rc.1", channel: "main",
			replies: map[string]fakeReply{"npm view": {stderr: npmE404, err: errExit}},
			wantArgv: [][]string{
				{"npm", "version", "1.0.0-rc.1", "--userconfig", "<npmrc>", "--no-git-tag-version", "--allow-same-version"},
				{"npm", "view", "@getoutreach/client@1.0.0-rc.1", "version", "--userconfig", "<npmrc>", "--registry", "https://npm.pkg.github.com/"},
				{"npm", "publish", "<pkg>", "--userconfig", "<npmrc>", "--tag", "main", "--registry", "https://npm.pkg.github.com/"},
			},
			wantInfo: &forge.Published{Name: "npm package (@main dist-tag)"},
		},
		{
			name: "already published version skipped", pkg: scoped, npmrc: githubNpmrc, version: "1.0.0",
			replies: map[string]fakeReply{"npm view": {stdout: "1.0.0\n"}},
			wantArgv: [][]string{
				{"npm", "version", "1.0.0", "--userconfig", "<npmrc>", "--no-git-tag-version", "--allow-same-version"},
				{"npm", "view", "@getoutreach/client@1.0.0", "version", "--userconfig", "<npmrc>", "--registry", "https://npm.pkg.github.com/"},
			},
			wantInfo: &forge.Published{Name: "npm package (@latest dist-tag)"},
		},
		{
			// integration.test.js: 'Skip npm token verification if
			// "package.private" is true' and 'Create the package and skip
			// publish from a sub-directory ("package.private" is true)'.
			name: "private package skipped", pkg: `{"name": "@getoutreach/client", "private": true}`, version: "1.0.0",
			wantArgv: [][]string{
				{"npm", "version", "1.0.0", "--userconfig", "<npmrc>", "--no-git-tag-version", "--allow-same-version"},
			},
		},
		{
			// integration.test.js: "Throws error if NPM token is not
			// provided when targeting the default registry".
			name: "missing token error", pkg: `{"name": "client"}`, version: "1.0.0",
			wantErr: errNPMNoToken,
		},
		{
			// integration.test.js: "Throws error if NPM token is invalid
			// when targeting the default registry".
			name: "invalid token error", pkg: `{"name": "client"}`, env: []string{"NPM_TOKEN=wrong_token"}, version: "1.0.0",
			replies: map[string]fakeReply{"npm whoami": {stderr: "npm error code E401\n", err: errExit}},
			wantArgv: [][]string{
				{"npm", "whoami", "--userconfig", "<npmrc>", "--registry", officialNPMRegistry},
			},
			wantErr: errNPMInvalidToken,
		},
		{
			// integration.test.js: "Publish the package on a dist-tag".
			name: "default registry with NPM_TOKEN", pkg: `{"name": "client"}`, env: []string{"NPM_TOKEN=npm_token"},
			version: "1.0.0", channel: "next",
			replies: map[string]fakeReply{"npm view": {stderr: npmE404, err: errExit}},
			wantArgv: [][]string{
				{"npm", "whoami", "--userconfig", "<npmrc>", "--registry", officialNPMRegistry},
				{"npm", "version", "1.0.0", "--userconfig", "<npmrc>", "--no-git-tag-version", "--allow-same-version"},
				{"npm", "view", "client@1.0.0", "version", "--userconfig", "<npmrc>", "--registry", officialNPMRegistry},
				{"npm", "publish", "<pkg>", "--userconfig", "<npmrc>", "--tag", "next", "--registry", officialNPMRegistry},
			},
			wantInfo: &forge.Published{Name: "npm package (@next dist-tag)", URL: "https://www.npmjs.com/package/client/v/1.0.0"},
		},
		{
			name: "npm view failure", pkg: scoped, npmrc: githubNpmrc, version: "1.0.0",
			replies: map[string]fakeReply{"npm view": {stderr: "npm error code E401\n", err: errExit}},
			wantArgv: [][]string{
				{"npm", "version", "1.0.0", "--userconfig", "<npmrc>", "--no-git-tag-version", "--allow-same-version"},
				{"npm", "view", "@getoutreach/client@1.0.0", "version", "--userconfig", "<npmrc>", "--registry", "https://npm.pkg.github.com/"},
			},
			wantErr: errExit,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := newNPMFixture(t, pkgRoot, tc.pkg, tc.env...)
			if tc.npmrc != "" {
				writeFile(t, f.home, ".npmrc", tc.npmrc)
			}
			f.run.replies = tc.replies
			n, p := f.registry(pkgRoot), f.publisher(tc.version, tc.channel)
			err := n.prepare(context.Background(), p)
			var info *forge.Published
			if err == nil {
				info, err = n.publish(context.Background(), p)
			}
			if tc.wantErr != nil {
				assert.ErrorIs(t, err, tc.wantErr)
			} else {
				assert.NilError(t, err)
				assert.DeepEqual(t, info, tc.wantInfo)
			}
			pkgDir := filepath.Join(f.repo, pkgRoot)
			for _, argv := range tc.wantArgv {
				if i := slices.Index(argv, "<pkg>"); i >= 0 {
					argv[i] = pkgDir
				}
			}
			assert.DeepEqual(t, f.run.argvs(), tc.wantArgv)
			for _, c := range f.run.calls {
				wantDir, wantEnv := f.repo, f.env
				switch c.argv[1] {
				case "version":
					wantDir = pkgDir
				case "whoami":
					wantEnv = withoutEnv(f.env, "DEFAULT_NPM_REGISTRY")
				}
				assert.Equal(t, c.dir, wantDir, c.argv)
				assert.DeepEqual(t, c.env, wantEnv)
			}
		})
	}
}

func TestNPMNpmrcHasNoToken(t *testing.T) {
	f := newNPMFixture(t, "", `{"name": "client"}`, "NPM_TOKEN=s3cr3t-value")
	f.run.replies = map[string]fakeReply{"npm view": {stderr: npmE404, err: errExit}}
	n, p := f.registry(""), f.publisher("1.0.0", "")
	assert.NilError(t, n.prepare(context.Background(), p))
	_, err := n.publish(context.Background(), p)
	assert.NilError(t, err)
	for _, c := range f.run.calls {
		assert.Equal(t, c.npmrc, "//registry.npmjs.org/:_authToken = ${NPM_TOKEN}")
	}
	assert.Assert(t, !strings.Contains(f.log.String(), "s3cr3t-value"))
}

func TestNPMExists(t *testing.T) {
	tests := []struct {
		name  string
		pkg   string
		reply fakeReply
		want  bool
		calls int
	}{
		{name: "published", pkg: `{"name": "@getoutreach/client"}`, reply: fakeReply{stdout: "1.0.0\n"}, want: true, calls: 1},
		{name: "missing", pkg: `{"name": "@getoutreach/client"}`, reply: fakeReply{stderr: npmE404, err: errExit}, calls: 1},
		{name: "private", pkg: `{"name": "@getoutreach/client", "private": true}`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := newNPMFixture(t, "", tc.pkg)
			writeFile(t, f.home, ".npmrc", githubNpmrc)
			f.run.replies = map[string]fakeReply{"npm view": tc.reply}
			got, err := f.registry("").exists(context.Background(), f.publisher("1.0.0", ""))
			assert.NilError(t, err)
			assert.Equal(t, got, tc.want)
			assert.Equal(t, len(f.run.calls), tc.calls)
		})
	}
}

// TestNPMPrepareRealNPM runs npm version with the production runner, as
// prepare.test.js "Update package.json" and "Updade package.json and
// package-lock.json in a sub-directory" do. It is skipped without npm.
func TestNPMPrepareRealNPM(t *testing.T) {
	if _, err := exec.LookPath("npm"); err != nil {
		t.Skip("npm is not installed")
	}
	// npm can be a shim that needs other tools on PATH, such as bash.
	f := newNPMFixture(t, "dist", `{"name": "@getoutreach/client", "version": "0.0.0-dev", "private": true}`,
		"PATH="+os.Getenv("PATH"))
	writeFile(t, f.repo, "dist/package-lock.json",
		`{"name": "@getoutreach/client", "version": "0.0.0-dev", "lockfileVersion": 3, "packages": {"": {"version": "0.0.0-dev"}}}`)
	n := &npmRegistry{plugin: &spec.NPMPlugin{PkgRoot: "dist"}, run: execCommand}
	assert.NilError(t, n.prepare(context.Background(), f.publisher("1.0.0", "")))
	for _, file := range []string{"package.json", "package-lock.json"} {
		b, err := os.ReadFile(filepath.Join(f.repo, "dist", file))
		assert.NilError(t, err)
		var got struct {
			Version string `json:"version"`
		}
		assert.NilError(t, json.Unmarshal(b, &got))
		assert.Equal(t, got.Version, "1.0.0", file)
	}
	assert.Assert(t, strings.Contains(f.log.String(), "Write version 1.0.0 to package.json in "+filepath.Join(f.repo, "dist")))
}
