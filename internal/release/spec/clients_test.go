// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Tests for the client version files.

package spec_test

import (
	"testing"

	"github.com/getoutreach/devbase/v2/internal/release/spec"
	"gotest.tools/v3/assert"
)

func TestClientVersionFiles(t *testing.T) {
	cfg := &spec.Config{Plugins: []spec.Plugin{
		{NPM: &spec.NPMPlugin{PkgRoot: "./api/clients/node/"}},
		{NPM: &spec.NPMPlugin{}},
		{NPM: &spec.NPMPlugin{PkgRoot: "/abs"}},
	}}
	assert.DeepEqual(t, cfg.ClientVersionFiles(), []string{
		"api/clients/node/package.json", "api/clients/node/package-lock.json", "api/clients/node/npm-shrinkwrap.json",
		"package.json", "package-lock.json", "npm-shrinkwrap.json",
	})
	for file, want := range map[string]bool{
		"api/clients/node/package.json":          true,
		"./api/clients/node/npm-shrinkwrap.json": true,
		"package.json":                           true,
		"api/clients/node/src/index.ts":          false,
		"abs/package-lock.json":                  false,
		"go.mod":                                 false,
	} {
		assert.Equal(t, cfg.IsClientVersionFile(file), want, file)
	}
}

func TestDropClientVersionFiles(t *testing.T) {
	commit := &spec.CommitPlugin{Files: []string{
		"CHANGELOG.md",
		"api/clients/node/package.json",
		"./api/clients/node/package-lock.json",
		"api/clients/*/npm-shrinkwrap.json",
		"api/clients",
	}}
	defaults := &spec.CommitPlugin{}
	cfg := &spec.Config{Plugins: []spec.Plugin{
		{NPM: &spec.NPMPlugin{PkgRoot: "./api/clients/node"}},
		{Commit: commit},
		{Commit: defaults},
	}}
	warnings := cfg.DropClientVersionFiles()
	assert.DeepEqual(t, warnings, []string{
		`the release does not commit the client version file "api/clients/node/package.json"; ` +
			"remove it from the release commit files",
		`the release does not commit the client version file "./api/clients/node/package-lock.json"; ` +
			"remove it from the release commit files",
		`the release does not commit the client version file "api/clients/*/npm-shrinkwrap.json"; ` +
			"remove it from the release commit files",
	})
	// The folder glob stays: the release commit leaves out the client
	// version files in it.
	assert.DeepEqual(t, commit.Files, []string{"CHANGELOG.md", "api/clients"})
	// The default files of a commit plugin without files stay, and the
	// release commit leaves the client version files out of them.
	assert.Assert(t, defaults.Files == nil)

	// Only client version files leave an empty list, which commits
	// nothing.
	only := &spec.CommitPlugin{Files: []string{"api/clients/node/package.json"}}
	cfg.Plugins[1].Commit = only
	cfg.DropClientVersionFiles()
	assert.Assert(t, only.Files != nil && len(only.Files) == 0)
}
