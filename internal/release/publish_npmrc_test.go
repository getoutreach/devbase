// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Tests for the npm config, registry and auth helpers,
// ported from the unit tests of @semantic-release/npm 13.2.0.

package release

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/getoutreach/gobox/pkg/exec/execstub"
	"gotest.tools/v3/assert"
)

// TestNPMDistTag ports test/get-channel.test.js.
func TestNPMDistTag(t *testing.T) {
	// "Get default channel"
	assert.Equal(t, npmDistTag(""), "latest")
	// "Get passed channel if valid"
	assert.Equal(t, npmDistTag("next"), "next")
	// 'Prefix channel with "release-" if invalid'
	assert.Equal(t, npmDistTag("1.x"), "release-1.x")
	assert.Equal(t, npmDistTag("1.0.0"), "release-1.0.0")
	// The prerelease branch channel of the stencil templates.
	assert.Equal(t, npmDistTag("main"), "main")
}

// TestReadNPMPackage ports test/get-pkg.test.js.
func TestReadNPMPackage(t *testing.T) {
	t.Run("Verify name and version then return parsed package.json", func(t *testing.T) {
		cwd := t.TempDir()
		writeFile(t, cwd, "package.json", `{"name": "package", "version": "0.0.0"}`)
		pkg, err := readNPMPackage(cwd)
		assert.NilError(t, err)
		assert.Equal(t, pkg.Name, "package")
	})
	t.Run("Verify name and version then return parsed package.json from a sub-directory", func(t *testing.T) {
		f := newNPMFixture(t, "dist", `{"name": "package", "version": "0.0.0"}`)
		s, err := f.registry("dist").open(t.Context(), f.publisher("1.0.0", ""), false)
		assert.ErrorIs(t, err, errNPMNoToken) // The package is found; it has no auth.
		assert.Assert(t, s == nil)
		pkg, err := readNPMPackage(filepath.Join(f.repo, "dist"))
		assert.NilError(t, err)
		assert.Equal(t, pkg.Name, "package")
	})
	t.Run("Throw error if missing package.json", func(t *testing.T) {
		_, err := readNPMPackage(t.TempDir())
		assert.ErrorIs(t, err, errNPMNoPkg)
	})
	t.Run("Throw error if missing package name", func(t *testing.T) {
		cwd := t.TempDir()
		writeFile(t, cwd, "package.json", `{"version": "0.0.0"}`)
		_, err := readNPMPackage(cwd)
		assert.ErrorIs(t, err, errNPMNoPkgName)
	})
	t.Run("Throw error if package.json is malformed", func(t *testing.T) {
		cwd := t.TempDir()
		writeFile(t, cwd, "package.json", "{name: 'package',}")
		_, err := readNPMPackage(cwd)
		assert.ErrorContains(t, err, "parse ")
	})
}

// TestNPMRegistryURL ports test/get-registry.test.js.
func TestNPMRegistryURL(t *testing.T) {
	registry := func(t *testing.T, cwd string, pkg *npmPackage, env ...string) string {
		t.Helper()
		env = append([]string{"HOME=" + t.TempDir()}, env...)
		cfg, _ := npmrcConfig(cwd, env)
		return npmRegistryURL(pkg, cfg, env)
	}
	t.Run("Get default registry", func(t *testing.T) {
		assert.Equal(t, registry(t, t.TempDir(), &npmPackage{Name: "package-name"}), officialNPMRegistry)
	})
	t.Run(`Get the registry configured in ".npmrc" and normalize trailing slash`, func(t *testing.T) {
		cwd := t.TempDir()
		writeFile(t, cwd, ".npmrc", "registry = https://custom1.registry.com")
		assert.Equal(t, registry(t, cwd, &npmPackage{Name: "package-name"}), "https://custom1.registry.com/")
	})
	t.Run(`Get the registry configured from "publishConfig"`, func(t *testing.T) {
		cwd := t.TempDir()
		writeFile(t, cwd, ".npmrc", "registry = https://custom2.registry.com")
		pkg := &npmPackage{Name: "package-name"}
		pkg.PublishConfig.Registry = "https://custom3.registry.com/"
		assert.Equal(t, registry(t, cwd, pkg), "https://custom3.registry.com/")
	})
	t.Run(`Get the registry configured in "NPM_CONFIG_REGISTRY"`, func(t *testing.T) {
		got := registry(t, t.TempDir(), &npmPackage{Name: "package-name"}, "NPM_CONFIG_REGISTRY=https://custom1.registry.com/")
		assert.Equal(t, got, "https://custom1.registry.com/")
	})
	t.Run(`Get the registry configured in ".npmrc" for scoped package`, func(t *testing.T) {
		cwd := t.TempDir()
		writeFile(t, cwd, ".npmrc", "@scope:registry = https://custom3.registry.com")
		assert.Equal(t, registry(t, cwd, &npmPackage{Name: "@scope/package-name"}), "https://custom3.registry.com/")
	})
	t.Run(`Get the registry configured via "NPM_CONFIG_USERCONFIG" for scoped package`, func(t *testing.T) {
		cwd := t.TempDir()
		writeFile(t, cwd, ".custom-npmrc", "@scope:registry = https://custom4.registry.com")
		got := registry(t, cwd, &npmPackage{Name: "@scope/package-name"},
			"NPM_CONFIG_USERCONFIG="+filepath.Join(cwd, ".custom-npmrc"))
		assert.Equal(t, got, "https://custom4.registry.com/")
	})
	// integration.test.js: 'Verify npm auth and package with
	// "npm_config_registry" env var set by yarn'. rc reads it as the key
	// "config_registry", so it does not change the registry.
	t.Run("npm_config_registry set by yarn", func(t *testing.T) {
		got := registry(t, t.TempDir(), &npmPackage{Name: "package-name"}, "npm_config_registry=https://registry.yarnpkg.com")
		assert.Equal(t, got, officialNPMRegistry)
	})
}

// TestNPMReleaseInfo ports test/get-release-info.test.js.
func TestNPMReleaseInfo(t *testing.T) {
	t.Run("Default registry and scoped module", func(t *testing.T) {
		got := npmReleaseInfo("@scope/module", "1.0.0", "latest", officialNPMRegistry, nil)
		assert.Equal(t, got.Name, "npm package (@latest dist-tag)")
		assert.Equal(t, got.URL, "https://www.npmjs.com/package/@scope/module/v/1.0.0")
	})
	t.Run("Custom registry and scoped module", func(t *testing.T) {
		got := npmReleaseInfo("@scope/module", "1.0.0", "latest", "https://custom.registry.org/", nil)
		assert.Equal(t, got.Name, "npm package (@latest dist-tag)")
		assert.Equal(t, got.URL, "")
	})
	// integration.test.js: "Publish the package on a dist-tag" sets
	// DEFAULT_NPM_REGISTRY to the test registry.
	t.Run("DEFAULT_NPM_REGISTRY", func(t *testing.T) {
		got := npmReleaseInfo("publish-tag", "1.0.0", "next", "http://localhost:4873/", []string{"DEFAULT_NPM_REGISTRY=http://localhost:4873"})
		assert.Equal(t, got.URL, "https://www.npmjs.com/package/publish-tag/v/1.0.0")
	})
}

// TestSetNpmrcAuth ports test/set-npmrc-auth.test.js. HOME is a
// temporary folder, and cwd is the repository.

func TestSetNpmrcAuth(t *testing.T) {
	type result struct {
		npmrc string
		log   string
		err   error
	}
	const custom = "http://custom.registry.com"
	setup := func(t *testing.T, registry, homeRC, cwdRC string, env ...string) (*npmFixture, result) {
		t.Helper()
		f := newNPMFixture(t, "", "", env...)
		if homeRC != "" {
			writeFile(t, f.home, ".npmrc", homeRC)
		}
		if cwdRC != "" {
			writeFile(t, f.repo, ".npmrc", cwdRC)
		}
		p := f.publisher("1.0.0", "")
		s := &npmSession{registry: registry, env: f.env, tmp: t.TempDir()}
		s.npmrc = filepath.Join(s.tmp, ".npmrc")
		cfg, files := npmrcConfig(f.repo, f.env)
		err := setNpmrcAuth(p, s, cfg, files)
		b, _ := os.ReadFile(s.npmrc)
		return f, result{npmrc: string(b), log: f.log.String(), err: err}
	}
	t.Run(`Set auth with "NPM_TOKEN"`, func(t *testing.T) {
		_, r := setup(t, custom, "", "", "NPM_TOKEN=npm_token")
		assert.NilError(t, r.err)
		assert.Equal(t, r.npmrc, "//custom.registry.com/:_authToken = ${NPM_TOKEN}")
		assert.Assert(t, strings.Contains(r.log, "Wrote NPM_TOKEN to "), r.log)
		assert.Assert(t, !strings.Contains(r.log, "npm_token"))
	})
	t.Run(`Preserve home ".npmrc"`, func(t *testing.T) {
		f, r := setup(t, custom, "home_config = test", "", "NPM_TOKEN=npm_token")
		assert.NilError(t, r.err)
		assert.Equal(t, r.npmrc, "home_config = test\n//custom.registry.com/:_authToken = ${NPM_TOKEN}")
		assert.Assert(t, strings.Contains(r.log, "Reading npm config from "+filepath.Join(f.home, ".npmrc")+"\n"), r.log)
	})
	t.Run(`Preserve home and local ".npmrc"`, func(t *testing.T) {
		f, r := setup(t, custom, "home_config = test", "cwd_config = test", "NPM_TOKEN=npm_token")
		assert.NilError(t, r.err)
		assert.Equal(t, r.npmrc, "home_config = test\ncwd_config = test\n//custom.registry.com/:_authToken = ${NPM_TOKEN}")
		want := "Reading npm config from " + filepath.Join(f.home, ".npmrc") + ", " + filepath.Join(f.repo, ".npmrc") + "\n"
		assert.Assert(t, strings.Contains(r.log, want), r.log)
	})
	t.Run(`Preserve all ".npmrc" if auth is already configured`, func(t *testing.T) {
		_, r := setup(t, custom, "home_config = test", "//custom.registry.com/:_authToken = ${NPM_TOKEN}")
		assert.NilError(t, r.err)
		assert.Equal(t, r.npmrc, "home_config = test\n//custom.registry.com/:_authToken = ${NPM_TOKEN}")
		assert.Assert(t, !strings.Contains(r.log, "Wrote NPM_TOKEN"))
	})
	t.Run(`Preserve ".npmrc" if auth is already configured for a scoped package`, func(t *testing.T) {
		_, r := setup(t, custom, "home_config = test",
			"@scope:registry=http://custom.registry.com\n//custom.registry.com/:_authToken = ${NPM_TOKEN}")
		assert.NilError(t, r.err)
		assert.Equal(t, r.npmrc,
			"home_config = test\n@scope:registry=http://custom.registry.com\n//custom.registry.com/:_authToken = ${NPM_TOKEN}")
	})
	t.Run(`Throw error if "NPM_TOKEN" is missing`, func(t *testing.T) {
		_, r := setup(t, custom, "", "")
		assert.ErrorIs(t, r.err, errNPMNoToken)
	})
	t.Run(`Emulate npm config resolution if "NPM_CONFIG_USERCONFIG" is set`, func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, dir, ".custom-npmrc", "//custom.registry.com/:_authToken = ${NPM_TOKEN}")
		_, r := setup(t, custom, "", "", "NPM_CONFIG_USERCONFIG="+filepath.Join(dir, ".custom-npmrc"))
		assert.NilError(t, r.err)
		assert.Equal(t, r.npmrc, "//custom.registry.com/:_authToken = ${NPM_TOKEN}")
		assert.Assert(t, strings.Contains(r.log, "Reading npm config from "+filepath.Join(dir, ".custom-npmrc")+"\n"), r.log)
	})
	t.Run("Prefer .npmrc over environment variables", func(t *testing.T) {
		_, r := setup(t, "http://registry.npmjs.org", "", "//registry.npmjs.org/:_authToken=npmrc_npm_token",
			"NPM_TOKEN=env_npm_token")
		assert.NilError(t, r.err)
		assert.Equal(t, r.npmrc, "//registry.npmjs.org/:_authToken=npmrc_npm_token")
		assert.Assert(t, !strings.Contains(r.log, "Wrote NPM_TOKEN"))
		assert.Assert(t, !strings.Contains(r.log, "npmrc_npm_token"))
	})
}

// TestNPMVerifyAuth ports test/verify-auth.test.js with stubs of npm
// whoami.
func TestNPMVerifyAuth(t *testing.T) {
	// open opens the package with npm whoami, which gives whoami when it
	// is not nil.
	open := func(t *testing.T, pkg string, whoami *execstub.Result, env ...string) (*npmFixture, error) {
		t.Helper()
		f := newNPMFixture(t, "", pkg, env...)
		if whoami != nil {
			f.cmds.expect(*whoami, npmWhoamiArgv...)
		}
		s, err := f.registry("").open(t.Context(), f.publisher("1.0.0", ""), true)
		if s != nil {
			s.close()
		}
		return f, err
	}
	t.Run("that the provided token is verified with `npm whoami` when trusted publishing is not established "+
		"for the official registry", func(t *testing.T) {
		me := cmdOK("me\n")
		f, err := open(t, `{"name": "client"}`, &me, "NPM_TOKEN=t", "DEFAULT_NPM_REGISTRY="+officialNPMRegistry, "foo=bar")
		assert.NilError(t, err)
		whoami := f.cmds.calls()[0].Cmd
		assert.DeepEqual(t, whoami.Env, []string{"HOME=" + f.home, "PATH=/usr/bin", "NPM_TOKEN=t", "foo=bar"})
		assert.Equal(t, whoami.Dir, f.repo)
	})
	t.Run("that the auth context for the official registry is considered invalid when no token is provided "+
		"and trusted publishing is not established", func(t *testing.T) {
		unauthorized := cmdFails("npm error code E401\n")
		_, err := open(t, `{"name": "client"}`, &unauthorized, "NPM_TOKEN=t")
		assert.ErrorIs(t, err, errNPMInvalidToken)
	})
	// "that a publish dry run is performed to validate token presence
	// when publishing to a custom registry" (and from a sub-directory):
	// verify-auth.js 13.2.0 does not call attemptPublishDryRun, and these
	// tests only check that it does not fail. A custom registry runs no
	// command.
	t.Run("custom registry runs no command", func(t *testing.T) {
		pkg := `{"name": "client", "publishConfig": {"registry": "https://other.registry.org"}}`
		_, err := open(t, pkg, nil, "NPM_TOKEN=t")
		assert.NilError(t, err)
	})
	t.Run("that errors from setting up auth bubble through this function", func(t *testing.T) {
		_, err := open(t, `{"name": "client"}`, nil)
		assert.ErrorIs(t, err, errNPMNoToken)
	})
	// integration.test.js: 'Skip npm token verification if
	// "package.private" is true'.
	t.Run("private package", func(t *testing.T) {
		_, err := open(t, `{"name": "client", "private": true}`, nil)
		assert.NilError(t, err)
	})
}

// TestNpmrcHelpers checks the ports of ini, registry-auth-token,
// nerf-dart and normalize-url on the values that the npm plugin uses.
func TestNpmrcHelpers(t *testing.T) {
	cfg := parseNpmrc("; comment\n# comment\nregistry = \"https://a.test/\"\n" +
		"key = value ; trailing\nescaped = a\\;b\nflag\nbool = true\n[section]\nhidden = 1\n")
	assert.DeepEqual(t, cfg, map[string]string{"registry": "https://a.test/", "key": "value", "escaped": "a;b"})
	assert.DeepEqual(t, parseNpmrc(`{"registry": "https://j.test/", "n": 1}`), map[string]string{"registry": "https://j.test/"})

	assert.Equal(t, nerfDart("https://npm.pkg.github.com"), "//npm.pkg.github.com/")
	assert.Equal(t, nerfDart("https://user:pw@Host.test:8080/a/b?q#h"), "//host.test:8080/a/")

	auth := map[string]string{"//npm.pkg.github.com/:_authToken": "${GH_TOKEN}"}
	assert.Assert(t, npmAuthConfigured("https://npm.pkg.github.com/", auth))
	assert.Assert(t, npmAuthConfigured("https://npm.pkg.github.com", auth))
	assert.Assert(t, !npmAuthConfigured("https://registry.npmjs.org/", auth))
	assert.Assert(t, npmAuthConfigured("https://r.test/sub/pkg", map[string]string{"//r.test/sub/:_authToken": "x"}))
	assert.Assert(t, npmAuthConfigured("https://r.test/", map[string]string{"//r.test/:username": "u", "//r.test/:_password": "cA=="}))
	assert.Assert(t, !npmAuthConfigured("https://r.test/", map[string]string{"//r.test/:username": "u"}))
	assert.Assert(t, npmAuthConfigured("https://r.test/", map[string]string{"_auth": "x"}))

	assert.Equal(t, normalizeRegistryURL("https://registry.npmjs.org/"), normalizeRegistryURL("HTTPS://www.Registry.npmjs.org:443"))
	assert.Assert(t, normalizeRegistryURL("https://registry.npmjs.org/") != normalizeRegistryURL("http://registry.npmjs.org/"))
}
