// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Tests for the release unit model: tag formats, path
// globs, Go module rules and validation.

package spec

import (
	"testing"

	"github.com/Masterminds/semver/v3"
	"gotest.tools/v3/assert"
)

func TestTagFormat(t *testing.T) {
	f := TagFormat("nodejs-widgets-client-v${version}")
	assert.Equal(t, f.Render("1.2.3"), "nodejs-widgets-client-v1.2.3")
	assert.Equal(t, f.Prefix(), "nodejs-widgets-client-v")
	v, ok := f.Version("nodejs-widgets-client-v1.2.3")
	assert.Assert(t, ok)
	assert.Equal(t, v, "1.2.3")
	_, ok = f.Version("nodejs-widgets-client-utils-v1.0.0")
	assert.Assert(t, !ok)
	v, ok = TagFormat("pkg-${version}").Version("pkg-utils-1.0.0")
	assert.Assert(t, ok, "the semver check is the caller's")
	assert.Equal(t, v, "utils-1.0.0")
	_, ok = TagFormat("api/v${version}-go").Version("api/v1.0.0")
	assert.Assert(t, !ok)
	v, ok = RootTagFormat.Version("v1.0.0")
	assert.Assert(t, ok)
	assert.Equal(t, v, "1.0.0")

	for _, good := range []TagFormat{"api/v${version}", "nodejs-widgets-client-v${version}", "pkg-${version}-x", "vendor-${version}"} {
		assert.NilError(t, good.Validate(), good)
	}
	for _, bad := range []TagFormat{
		"", "api", "${version}", "v${version}", "v1-${version}", "a-${version}-${version}",
		"a ${version}", "a..${version}", "a~${version}", "a/${version}.lock", "a-${version}-${x}", "a*${version}",
	} {
		assert.ErrorIs(t, bad.Validate(), ErrUnsupported, bad)
	}
}

func TestMatchPath(t *testing.T) {
	cases := []struct {
		glob, file string
		want       bool
	}{
		{"node/widgets-client", "node/widgets-client/src/index.ts", true},
		{"node/widgets-client", "node/widgets-clientx/x", false},
		{"node/**", "node/widgets-client/package.json", true},
		{"node/**", "nodes/x", false},
		{"**/*.md", "a/b/c.md", true},
		{"**/*.md", "c.md", true},
		{"*.md", "a/c.md", false},
		{".circleci/config.yml", ".circleci/config.yml", true},
		{"node/*/package.json", "node/widgets-client/package.json", true},
		{"node/*/package.json", "node/widgets-client/x/package.json", false},
		{"api/**/clients", "api/clients/node/package.json", true},
	}
	for _, tc := range cases {
		assert.Equal(t, MatchPath(tc.glob, tc.file), tc.want, "%s %s", tc.glob, tc.file)
	}
	for _, bad := range []string{"", "/a", "./a", "a/../b", "a//b", "!a", `a\b`, "a**", "a/[b"} {
		assert.ErrorIs(t, ValidatePathGlob(bad), ErrUnsupported, bad)
	}
	assert.NilError(t, ValidatePathGlob("node/**/src/*.ts"))
}

func TestGoTagPrefix(t *testing.T) {
	cases := []struct {
		dir, module, want string
	}{
		{"api", "github.com/acme/widgets/api", "api/"},
		{"pkg/v1/scope", "github.com/acme/widgets/pkg/v1/scope", "pkg/v1/scope/"},
		{"api", "github.com/acme/widgets/api/v2", "api/"},    // major branch layout
		{"api/v2", "github.com/acme/widgets/api/v2", "api/"}, // major subdirectory layout
	}
	for _, tc := range cases {
		got, err := GoTagPrefix(tc.dir, tc.module)
		assert.NilError(t, err)
		assert.Equal(t, got, tc.want, tc.module)
	}
	_, err := GoTagPrefix("api", "github.com/acme/other")
	assert.ErrorIs(t, err, ErrUnsupported)

	assert.Equal(t, GoMajorSuffix("x/api/v3"), "/v3")
	assert.Equal(t, GoMajorSuffix("x/api/v1"), "")
	assert.NilError(t, CheckGoMajor("x/api", semver.MustParse("1.9.0")))
	assert.NilError(t, CheckGoMajor("x/api", semver.MustParse("0.3.0")))
	assert.ErrorIs(t, CheckGoMajor("x/api", semver.MustParse("2.0.0")), ErrUnsupported)
	assert.NilError(t, CheckGoMajor("x/api/v2", semver.MustParse("2.0.0")))
	assert.ErrorIs(t, CheckGoMajor("x/api/v2", semver.MustParse("3.0.0")), ErrUnsupported)
	assert.ErrorIs(t, CheckGoMajor("x/api/v2", semver.MustParse("1.0.0")), ErrUnsupported)
}

func TestUnitDefaults(t *testing.T) {
	u := Unit{Kind: UnitGoModule, Dir: "api", Go: &GoUnit{ModulePath: "github.com/acme/widgets/api"}}
	u.SetDefaults()
	assert.DeepEqual(t, u, Unit{
		Name: "api", Kind: UnitGoModule, Dir: "api", Paths: []string{"api"}, TagFormat: "api/v${version}",
		Strategy: StrategyMirror, Go: &GoUnit{ModulePath: "github.com/acme/widgets/api", PinRoot: GoPinAuto},
	})
	n := Unit{Kind: UnitNPM, Dir: "node/widgets-client", TagFormat: "nodejs-widgets-client-v${version}"}
	n.SetDefaults()
	assert.Equal(t, n.Name, "widgets-client")
	assert.Equal(t, n.Strategy, StrategyIndependent)
	assert.DeepEqual(t, n.NPM, &NPMPlugin{PkgRoot: "node/widgets-client"})
}

// unitConfig returns a valid config with a forge, an npm unit and a Go
// unit, after f changes it.
func unitConfig(f func(c *Config)) *Config {
	c := &Config{
		Branches: []Branch{{Name: "main"}},
		Plugins:  []Plugin{{Forge: &ForgePlugin{Provider: ForgeGitHub}}},
		Units: []Unit{
			{Kind: UnitNPM, Dir: "node/widgets-client", TagFormat: "nodejs-widgets-client-v${version}", ForgeRelease: true},
			{Kind: UnitGoModule, Dir: "api", Go: &GoUnit{ModulePath: "github.com/acme/widgets/api"}},
		},
		RootPaths: RootPaths{Exclude: []string{"node/**"}, Shared: []string{".circleci/config.yml"}},
	}
	for i := range c.Units {
		c.Units[i].SetDefaults()
	}
	if f != nil {
		f(c)
	}
	return c
}

func TestValidateUnits(t *testing.T) {
	_, err := unitConfig(nil).Validate()
	assert.NilError(t, err)

	bad := map[string]func(c *Config){
		"root name":          func(c *Config) { c.Units[0].Name = RootUnitName },
		"duplicate name":     func(c *Config) { c.Units[1].Name = "widgets-client" },
		"bad dir":            func(c *Config) { c.Units[0].Dir = "../x" },
		"bad path":           func(c *Config) { c.Units[0].Exclude = []string{"/x"} },
		"bad root path":      func(c *Config) { c.RootPaths.Shared = []string{"a/../b"} },
		"duplicate format":   func(c *Config) { c.Units[0].TagFormat = c.Units[1].TagFormat },
		"root-like format":   func(c *Config) { c.Units[0].TagFormat = "v${version}" },
		"bad kind":           func(c *Config) { c.Units[0].Kind = "ruby" },
		"bad strategy":       func(c *Config) { c.Units[0].Strategy = "always" },
		"npm mirror":         func(c *Config) { c.Units[0].Strategy = StrategyMirror },
		"mirror major":       func(c *Config) { c.Units[1].AllowMajorVersions = true },
		"mirror excluded":    func(c *Config) { c.RootPaths.Exclude = append(c.RootPaths.Exclude, "api/**") },
		"forge without root": func(c *Config) { c.Plugins = nil },
		"go tag format":      func(c *Config) { c.Units[1].TagFormat = "api-v${version}" },
		"go without module":  func(c *Config) { c.Units[1].Go.ModulePath = "" },
		"go bad pin":         func(c *Config) { c.Units[1].Go.PinRoot = "always" },
		"go module path":     func(c *Config) { c.Units[1].Go.ModulePath = "github.com/acme/widgets/other" },
		"npm with go":        func(c *Config) { c.Units[0].Go = &GoUnit{} },
		"go with npm":        func(c *Config) { c.Units[1].NPM = &NPMPlugin{PkgRoot: "api"} },
		"generic with npm":   func(c *Config) { c.Units[0].Kind = UnitGeneric },
		"npm without root":   func(c *Config) { c.Units[0].NPM.PkgRoot = "" },
		"root rules alone":   func(c *Config) { c.Units = nil },
		"root npm install":   func(c *Config) { c.Plugins = append(c.Plugins, Plugin{NPM: &NPMPlugin{Install: "yarn"}}) },
	}
	for name, f := range bad {
		_, err := unitConfig(f).Validate()
		assert.ErrorIs(t, err, ErrUnsupported, name)
	}
}
