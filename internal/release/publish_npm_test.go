// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Tests for the npm registry, with cases ported from
// @semantic-release/npm 13.2.0 (test/*.test.js), and stubs of the npm
// commands.

package release

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/getoutreach/devbase/v2/internal/release/forge"
	"github.com/getoutreach/devbase/v2/internal/release/spec"
	"github.com/getoutreach/gobox/pkg/exec"
	"github.com/getoutreach/gobox/pkg/exec/execstub"
	"gotest.tools/v3/assert"
)

// npmE404 is the stderr of npm 10 for a missing version.
const npmE404 = "npm error code E404\nnpm error 404 No match found for version 1.0.0\n"

// npmFixture is a repository with a package.json, an isolated HOME and
// stubs of the registry commands.
type npmFixture struct {
	t    *testing.T
	repo string
	home string
	env  []string
	cmds *cmdStubs
	log  bytes.Buffer
}

// newNPMFixture makes a repository with pkgJSON at pkgRoot, and an env
// with HOME and the extra variables.
func newNPMFixture(t *testing.T, pkgRoot, pkgJSON string, extraEnv ...string) *npmFixture {
	t.Helper()
	f := &npmFixture{t: t, repo: t.TempDir(), home: t.TempDir(), cmds: newCmdStubs(t)}
	if pkgJSON != "" {
		writeFile(t, f.repo, filepath.Join(pkgRoot, "package.json"), pkgJSON)
	}
	f.env = append([]string{"HOME=" + f.home, "PATH=/usr/bin"}, extraEnv...)
	return f
}

// publisher returns the registry target of version on channel, with the
// stubs.
func (f *npmFixture) publisher(version, channel string) *regTarget {
	return &regTarget{dir: f.repo, version: version, channel: channel, env: f.env, exec: f.cmds.spy, log: &f.log}
}

// registry returns the npm registry of pkgRoot.
func (f *npmFixture) registry(pkgRoot string) *npmRegistry {
	return &npmRegistry{plugin: &spec.NPMPlugin{PkgRoot: pkgRoot}}
}

// githubNpmrc is the ~/.npmrc of Outreach CI: the scope registry and a
// token from an environment variable.
const githubNpmrc = "@getoutreach:registry=https://npm.pkg.github.com\n//npm.pkg.github.com/:_authToken=${GH_TOKEN}\n"

// npmVersionArgv is the argv of npm version.
func npmVersionArgv(version string) []string {
	return []string{"npm", "version", version, "--userconfig", execstub.AnyArg, "--no-git-tag-version", "--allow-same-version"}
}

// npmViewArgv is the argv of npm view for pkgVersion on registry.
func npmViewArgv(pkgVersion, registry string) []string {
	return []string{"npm", "view", pkgVersion, "version", "--userconfig", execstub.AnyArg, "--registry", registry}
}

// npmPublishArgv is the argv of npm publish of dir with distTag on
// registry.
func npmPublishArgv(dir, distTag, registry string) []string {
	return []string{"npm", "publish", dir, "--userconfig", execstub.AnyArg, "--tag", distTag, "--registry", registry}
}

// npmWhoamiArgv is the argv of npm whoami on the official registry.
var npmWhoamiArgv = []string{"npm", "whoami", "--userconfig", execstub.AnyArg, "--registry", officialNPMRegistry}

// TestNPMPublishScenarios checks the argv, dir and env of the npm
// commands in prepare and publish.
func TestNPMPublishScenarios(t *testing.T) {
	const pkgRoot = "api/clients/node"
	const scoped = `{"name": "@getoutreach/client", "version": "0.0.0-development"}`
	const github = "https://npm.pkg.github.com/"
	// pkg is the package folder in npm publish.
	const pkg = "<pkg>"
	tests := []struct {
		name     string
		pkg      string
		npmrc    string
		env      []string
		version  string
		channel  string
		cmds     []stubCmd
		wantInfo *forge.Published
		wantErr  error
	}{
		{
			// Scenario 10 of the RFC: npm version, then npm publish with
			// dist-tag latest, to the scope registry in ~/.npmrc.
			name: "first publish", pkg: scoped, npmrc: githubNpmrc, version: "1.0.0",
			cmds: []stubCmd{
				{npmVersionArgv("1.0.0"), cmdOK("")},
				{npmViewArgv("@getoutreach/client@1.0.0", github), cmdFails(npmE404)},
				{npmPublishArgv(pkg, "latest", github), cmdOK("")},
			},
			wantInfo: &forge.Published{Name: "npm package (@latest dist-tag)"},
		},
		{
			name: "first publish with old npm", pkg: scoped, npmrc: githubNpmrc, version: "1.0.0",
			cmds: []stubCmd{
				{npmVersionArgv("1.0.0"), cmdOK("")},
				{npmViewArgv("@getoutreach/client@1.0.0", github), cmdOK("")},
				{npmPublishArgv(pkg, "latest", github), cmdOK("")},
			},
			wantInfo: &forge.Published{Name: "npm package (@latest dist-tag)"},
		},
		{
			name: "prerelease channel dist-tag", pkg: scoped, npmrc: githubNpmrc, version: "1.0.0-rc.1", channel: "main",
			cmds: []stubCmd{
				{npmVersionArgv("1.0.0-rc.1"), cmdOK("")},
				{npmViewArgv("@getoutreach/client@1.0.0-rc.1", github), cmdFails(npmE404)},
				{npmPublishArgv(pkg, "main", github), cmdOK("")},
			},
			wantInfo: &forge.Published{Name: "npm package (@main dist-tag)"},
		},
		{
			name: "already published version skipped", pkg: scoped, npmrc: githubNpmrc, version: "1.0.0",
			cmds: []stubCmd{
				{npmVersionArgv("1.0.0"), cmdOK("")},
				{npmViewArgv("@getoutreach/client@1.0.0", github), cmdOK("1.0.0\n")},
			},
			wantInfo: &forge.Published{Name: "npm package (@latest dist-tag)"},
		},
		{
			// integration.test.js: 'Skip npm token verification if
			// "package.private" is true' and 'Create the package and skip
			// publish from a sub-directory ("package.private" is true)'.
			name: "private package skipped", pkg: `{"name": "@getoutreach/client", "private": true}`, version: "1.0.0",
			cmds: []stubCmd{{npmVersionArgv("1.0.0"), cmdOK("")}},
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
			cmds:    []stubCmd{{npmWhoamiArgv, cmdFails("npm error code E401\n")}},
			wantErr: errNPMInvalidToken,
		},
		{
			// integration.test.js: "Publish the package on a dist-tag".
			name: "default registry with NPM_TOKEN", pkg: `{"name": "client"}`, env: []string{"NPM_TOKEN=npm_token"},
			version: "1.0.0", channel: "next",
			cmds: []stubCmd{
				{npmWhoamiArgv, cmdOK("")},
				{npmVersionArgv("1.0.0"), cmdOK("")},
				{npmViewArgv("client@1.0.0", officialNPMRegistry), cmdFails(npmE404)},
				{npmPublishArgv(pkg, "next", officialNPMRegistry), cmdOK("")},
			},
			wantInfo: &forge.Published{Name: "npm package (@next dist-tag)", URL: "https://www.npmjs.com/package/client/v/1.0.0"},
		},
		{
			name: "npm view failure", pkg: scoped, npmrc: githubNpmrc, version: "1.0.0",
			cmds: []stubCmd{
				{npmVersionArgv("1.0.0"), cmdOK("")},
				{npmViewArgv("@getoutreach/client@1.0.0", github), cmdFails("npm error code E401\n")},
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
			pkgDir := filepath.Join(f.repo, pkgRoot)
			for _, c := range tc.cmds {
				argv := slices.Clone(c.argv)
				if i := slices.Index(argv, pkg); i >= 0 {
					argv[i] = pkgDir
				}
				f.cmds.expect(c.res, argv...)
			}
			n, p := f.registry(pkgRoot), f.publisher(tc.version, tc.channel)
			err := n.prepare(context.Background(), p)
			var info *forge.Published
			if err == nil {
				info, err = n.publish(context.Background(), p)
			}
			if tc.wantErr != nil {
				assertErrIs(t, err, tc.wantErr)
			} else {
				assert.NilError(t, err)
				assert.DeepEqual(t, info, tc.wantInfo)
			}
			for _, c := range f.cmds.calls() {
				wantDir, wantEnv := f.repo, f.env
				switch c.Args[0] {
				case "version":
					wantDir = pkgDir
				case "whoami":
					wantEnv = withoutEnv(f.env, "DEFAULT_NPM_REGISTRY")
				}
				assert.Equal(t, c.Cmd.Dir, wantDir, c.Args)
				assert.DeepEqual(t, c.Cmd.Env, wantEnv)
			}
		})
	}
}

// TestNPMNpmrcHasNoToken checks that the temporary npm config of each
// command names NPM_TOKEN, and that no file or log has its value.
func TestNPMNpmrcHasNoToken(t *testing.T) {
	f := newNPMFixture(t, "", `{"name": "client"}`, "NPM_TOKEN=s3cr3t-value")
	var npmrcs []string
	// readNpmrc keeps the npm config of the command, and then gives
	// stderr with code.
	readNpmrc := func(stderr string, code int) execstub.Result {
		return execstub.Result{Func: func(_ context.Context, cmd *exec.Cmd, _ io.Reader, _, _ io.Writer) error {
			b, err := os.ReadFile(cmd.Args[slices.Index(cmd.Args, "--userconfig")+1])
			if err != nil {
				return err
			}
			npmrcs = append(npmrcs, string(b))
			return nil
		}, Stderr: stderr, ExitCode: code}
	}
	f.cmds.expect(readNpmrc("", 0), npmWhoamiArgv...)
	f.cmds.expect(readNpmrc("", 0), npmVersionArgv("1.0.0")...)
	f.cmds.expect(readNpmrc(npmE404, 1), npmViewArgv("client@1.0.0", officialNPMRegistry)...)
	f.cmds.expect(readNpmrc("", 0), npmPublishArgv(f.repo, "latest", officialNPMRegistry)...)
	n, p := f.registry(""), f.publisher("1.0.0", "")
	assert.NilError(t, n.prepare(context.Background(), p))
	_, err := n.publish(context.Background(), p)
	assert.NilError(t, err)
	assert.Equal(t, len(npmrcs), 4)
	for _, npmrc := range npmrcs {
		assert.Equal(t, npmrc, "//registry.npmjs.org/:_authToken = ${NPM_TOKEN}")
	}
	assert.Assert(t, !strings.Contains(f.log.String(), "s3cr3t-value"))
}

// TestNPMInstall checks that prepare runs the install command in the
// package folder, after the npm config and before npm version, with the
// secrets masked in the log.
func TestNPMInstall(t *testing.T) {
	pkg := `{"name": "@acme/widgets", "publishConfig": {"registry": "https://npm.pkg.github.com"}}`
	f := newNPMFixture(t, "node/widgets", pkg, "NPM_TOKEN=s3cr3t-value")
	n, p := f.registry("node/widgets"), f.publisher("1.0.0", "")
	n.plugin.Install = `yarn install --frozen-lockfile --network-concurrency "1" --token s3cr3t-value`
	yarn := []string{"yarn", "install", "--frozen-lockfile", "--network-concurrency", "1", "--token", "s3cr3t-value"}
	f.cmds.expect(cmdOK("Done\n"), yarn...)
	f.cmds.expect(cmdOK(""), npmVersionArgv("1.0.0")...)
	assert.NilError(t, n.prepare(context.Background(), p))
	dir := filepath.Join(f.repo, "node", "widgets")
	install := f.cmds.calls()[0].Cmd
	assert.Equal(t, install.Dir, dir)
	assert.DeepEqual(t, install.Env, f.env)
	assert.Assert(t, strings.Contains(f.log.String(),
		"Run yarn install --frozen-lockfile --network-concurrency 1 --token [secure]\nDone\n"), f.log.String())
	assert.Assert(t, !strings.Contains(f.log.String(), "s3cr3t-value"))

	// A failed install stops prepare before npm version, and its error
	// has the masked command line and stderr.
	f.cmds = newCmdStubs(t)
	p = f.publisher("1.0.0", "")
	f.cmds.expect(cmdFails("error s3cr3t-value"), yarn...)
	err := n.prepare(context.Background(), p)
	assertErrIs(t, err, errExit)
	assert.ErrorContains(t, err, "npm install command: yarn install --frozen-lockfile --network-concurrency 1 "+
		"--token [secure]: exit status 1: error [secure]")

	// A command without a program, or with an open quote, is an error.
	for _, bad := range []string{" ", `yarn "install`} {
		n.plugin.Install = bad
		assert.Assert(t, n.prepare(context.Background(), p) != nil, bad)
	}
	_, err = npmInstallArgs("; echo")
	assert.ErrorIs(t, err, errEmptyHook)
}

func TestNPMExists(t *testing.T) {
	tests := []struct {
		name string
		pkg  string
		cmds []stubCmd
		want bool
	}{
		{
			name: "published", pkg: `{"name": "@getoutreach/client"}`, want: true,
			cmds: []stubCmd{{npmViewArgv("@getoutreach/client@1.0.0", "https://npm.pkg.github.com/"), cmdOK("1.0.0\n")}},
		},
		{
			name: "missing", pkg: `{"name": "@getoutreach/client"}`,
			cmds: []stubCmd{{npmViewArgv("@getoutreach/client@1.0.0", "https://npm.pkg.github.com/"), cmdFails(npmE404)}},
		},
		{name: "private", pkg: `{"name": "@getoutreach/client", "private": true}`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := newNPMFixture(t, "", tc.pkg)
			writeFile(t, f.home, ".npmrc", githubNpmrc)
			f.cmds.expectAll(tc.cmds)
			got, err := f.registry("").exists(context.Background(), f.publisher("1.0.0", ""))
			assert.NilError(t, err)
			assert.Equal(t, got, tc.want)
		})
	}
}

// TestNPMExistsChecksToken checks that exists verifies the token with npm
// whoami on the default registry, as prepare does, and fails before npm
// view, which gives E404 for a restricted package with a bad token.
func TestNPMExistsChecksToken(t *testing.T) {
	pkg := `{"name": "@acme/client"}`
	view := npmViewArgv("@acme/client@1.0.0", officialNPMRegistry)

	f := newNPMFixture(t, "", pkg, "NPM_TOKEN=bad")
	f.cmds.expect(cmdFails("npm error code E401\n"), npmWhoamiArgv...)
	_, err := f.registry("").exists(context.Background(), f.publisher("1.0.0", ""))
	assert.ErrorIs(t, err, errNPMInvalidToken)
	assert.Equal(t, len(f.cmds.calls()), 1, "exists runs no npm view")

	f = newNPMFixture(t, "", pkg, "NPM_TOKEN=good")
	f.cmds.expectAll([]stubCmd{{npmWhoamiArgv, cmdOK("acme\n")}, {view, cmdFails(npmE404)}})
	got, err := f.registry("").exists(context.Background(), f.publisher("1.0.0", ""))
	assert.NilError(t, err)
	assert.Assert(t, !got)
}

// TestNPMPrepareRealNPM runs npm version as a real process, as
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
	p := f.publisher("1.0.0", "")
	p.exec = nil
	assert.NilError(t, f.registry("dist").prepare(context.Background(), p))
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
