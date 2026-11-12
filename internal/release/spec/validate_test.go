// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Tests for the release model rules, without a loader.

package spec_test

import (
	"testing"

	"github.com/getoutreach/devbase/v2/internal/release/spec"
	"gotest.tools/v3/assert"
)

// validConfig returns a config that passes Validate: a release branch, an
// rc prerelease branch, one rule, every plugin kind, and one build.
func validConfig() *spec.Config {
	return &spec.Config{
		Branches: []spec.Branch{
			{Name: "main", Prerelease: "rc", Channel: "main"},
			{Name: "release"},
		},
		Rules: []spec.Rule{{Type: "feat", Scope: "core-*", Release: spec.BumpPatch}},
		Plugins: []spec.Plugin{
			{Notes: &spec.NotesPlugin{}},
			{Forge: &spec.ForgePlugin{Provider: spec.ForgeGitHub, Assets: []string{"dist/*.tar.gz"}}},
			{Commit: &spec.CommitPlugin{Files: []string{"package.json"}}},
			{NPM: &spec.NPMPlugin{}},
			{Orb: &spec.OrbPlugin{Name: "acme/orb", Path: "orb.yml"}},
			{Exec: &spec.ExecHooks{Publish: "./publish.sh ${nextRelease.version}"}},
			{Gem: &spec.GemPlugin{}},
		},
		Artifacts: &spec.Artifacts{
			Hooks:     []string{"make dep"},
			Builds:    []spec.Build{{ID: "app", Binary: "app", Main: ".", LDFlags: []string{"-X main.v={{ .Version }}"}}},
			Targets:   []spec.Target{spec.NewTarget("linux", "amd64")},
			Files:     []spec.ArchiveFile{{Src: "README*"}},
			Checksums: "checksums.txt",
		},
	}
}

func TestValidateAcceptsValidConfig(t *testing.T) {
	warnings, err := validConfig().Validate()
	assert.NilError(t, err)
	assert.Equal(t, len(warnings), 0)
}

func TestValidateWarnsAboutLowercaseDryRun(t *testing.T) {
	cfg := validConfig()
	cfg.Plugins = append(cfg.Plugins, spec.Plugin{Exec: &spec.ExecHooks{Prepare: "x ${options.dryrun}"}})
	warnings, err := cfg.Validate()
	assert.NilError(t, err)
	assert.Equal(t, len(warnings), 1)
}

func TestValidateFailsClosed(t *testing.T) {
	for name, change := range map[string]func(c *spec.Config){
		"no release branch":          func(c *spec.Config) { c.Branches = c.Branches[:1] },
		"two release branches":       func(c *spec.Config) { c.Branches = append(c.Branches, spec.Branch{Name: "next"}) },
		"duplicate branch":           func(c *spec.Config) { c.Branches[0].Name = "release" },
		"invalid branch name":        func(c *spec.Config) { c.Branches[1].Name = "~release" },
		"maintenance branch":         func(c *spec.Config) { c.Branches[1].Name = "1.x" },
		"release branch channel":     func(c *spec.Config) { c.Branches[1].Channel = "next" },
		"prerelease without channel": func(c *spec.Config) { c.Branches[0].Channel = "" },
		"numeric prerelease":         func(c *spec.Config) { c.Branches[0].Prerelease = "1" },
		"dotted prerelease":          func(c *spec.Config) { c.Branches[0].Prerelease = "rc.1" },
		"invalid prerelease":         func(c *spec.Config) { c.Branches[0].Prerelease = "r_c" },
		"two prerelease branches with one id": func(c *spec.Config) {
			c.Branches = append(c.Branches, spec.Branch{Name: "beta", Prerelease: "rc", Channel: "beta"})
		},
		"rule release":         func(c *spec.Config) { c.Rules[0].Release = "prerelease" },
		"rule without release": func(c *spec.Config) { c.Rules[0].Release = spec.BumpNone },
		"rule glob":            func(c *spec.Config) { c.Rules[0].Scope = "{a,b}" },
		"plugin with no capability": func(c *spec.Config) {
			c.Plugins = append(c.Plugins, spec.Plugin{})
		},
		"plugin with two capabilities": func(c *spec.Config) { c.Plugins[0].NPM = &spec.NPMPlugin{} },
		"unknown forge":                func(c *spec.Config) { c.Plugins[1].Forge.Provider = "gitlab" },
		"forge asset globstar":         func(c *spec.Config) { c.Plugins[1].Forge.Assets = []string{"dist/**"} },
		"commit file negation":         func(c *spec.Config) { c.Plugins[2].Commit.Files = []string{"!a"} },
		"orb without a name":           func(c *spec.Config) { c.Plugins[4].Orb.Name = "" },
		"exec command field":           func(c *spec.Config) { c.Plugins[5].Exec.Publish = "x ${nextRelease.notes}" },
		"exec lodash template":         func(c *spec.Config) { c.Plugins[5].Exec.Publish = "x <%= a %>" },
		"no builds":                    func(c *spec.Config) { c.Artifacts.Builds = nil },
		"no build id":                  func(c *spec.Config) { c.Artifacts.Builds[0].ID = "" },
		"no binary":                    func(c *spec.Config) { c.Artifacts.Builds[0].Binary = "" },
		"templated binary":             func(c *spec.Config) { c.Artifacts.Builds[0].Binary = "{{ .Version }}" },
		"no main":                      func(c *spec.Config) { c.Artifacts.Builds[0].Main = "" },
		"no ldflags":                   func(c *spec.Config) { c.Artifacts.Builds[0].LDFlags = nil },
		"ldflags field":                func(c *spec.Config) { c.Artifacts.Builds[0].LDFlags = []string{"{{ .Date }}"} },
		"env item":                     func(c *spec.Config) { c.Artifacts.Builds[0].Env = []string{"-X a"} },
		"env template":                 func(c *spec.Config) { c.Artifacts.Builds[0].Env = []string{"A={{ now }}"} },
		"duplicate build id": func(c *spec.Config) {
			c.Artifacts.Builds = append(c.Artifacts.Builds, spec.Build{ID: "app", Binary: "app2", Main: ".", LDFlags: []string{"-s"}})
		},
		"duplicate binary": func(c *spec.Config) {
			c.Artifacts.Builds = append(c.Artifacts.Builds, spec.Build{ID: "app2", Binary: "app", Main: ".", LDFlags: []string{"-s"}})
		},
		"duplicate target": func(c *spec.Config) {
			c.Artifacts.Targets = append(c.Artifacts.Targets, spec.NewTarget("linux", "amd64"))
		},
		"target without variant": func(c *spec.Config) { c.Artifacts.Targets[0].Goamd64 = "" },
		"unsupported target":     func(c *spec.Config) { c.Artifacts.Targets[0] = spec.NewTarget("windows", "mips") },
		"hook field":             func(c *spec.Config) { c.Artifacts.Hooks = []string{"make {{ .Commit }}"} },
		"archive src":            func(c *spec.Config) { c.Artifacts.Files[0].Src = "../x" },
		"archive src field":      func(c *spec.Config) { c.Artifacts.Files[0].Src = "{{ .Os }}" },
		"no checksums name":      func(c *spec.Config) { c.Artifacts.Checksums = "" },
		"checksums path":         func(c *spec.Config) { c.Artifacts.Checksums = "dist/checksums.txt" },
		"checksums template":     func(c *spec.Config) { c.Artifacts.Checksums = "{{ .Version }}.txt" },
	} {
		t.Run(name, func(t *testing.T) {
			cfg := validConfig()
			change(cfg)
			_, err := cfg.Validate()
			assert.ErrorIs(t, err, spec.ErrUnsupported)
		})
	}
}
