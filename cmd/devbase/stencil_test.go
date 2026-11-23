// Copyright 2026 Outreach Corporation. Licensed under the Apache License 2.0.

// Description: End-to-end tests for the "stencil" CLI subcommands.

package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/getoutreach/devbase/v2/internal/stencilupgrade"
	"gotest.tools/v3/assert"
)

func TestStencilLockDiffCommand(t *testing.T) {
	dir := t.TempDir()
	oldPath := filepath.Join(dir, "old.lock")
	newPath := filepath.Join(dir, "new.lock")
	oldLock := "version: v1.47.0\nmodules:\n" +
		"  - name: a\n    version: v1.0.0\n" +
		"  - name: u\n    version: unstable\n"
	newLock := "version: v1.47.0\nmodules:\n" +
		"  - name: a\n    version: v1.1.0\n" +
		"  - name: b\n    version: v0.1.0\n" +
		"  - name: u\n    version: unstable\n"
	assert.NilError(t, os.WriteFile(oldPath, []byte(oldLock), 0o600))
	assert.NilError(t, os.WriteFile(newPath, []byte(newLock), 0o600))

	stdout, _, err := runSubcommand(t, newStencilCommand(), "", "lock-diff", "--old", oldPath, "--new", newPath)
	assert.NilError(t, err)
	assert.Equal(t, stdout, `[{"module":"a","old":"v1.0.0","new":"v1.1.0"},`+
		`{"module":"b","old":"","new":"v0.1.0"},`+
		`{"module":"u","old":"unstable","new":"unstable","unstable":true}]`+"\n")
}

// fakeLister is a stencilupgrade.VersionLister that returns fixed data.
type fakeLister struct{}

func (fakeLister) Releases(context.Context, string) ([]stencilupgrade.Release, error) {
	return []stencilupgrade.Release{{Tag: "v1.0.0"}, {Tag: "v1.1.0"}}, nil
}

func (fakeLister) DefaultBranch(context.Context, string) (string, error) { return "main", nil }

func TestStencilPRBodyCommand(t *testing.T) {
	changes := `[{"module":"github.com/org/mod","old":"v1.0.0","new":"v1.1.0"}]`
	stdout, _, err := runSubcommand(t, newStencilCommandWithLister(fakeLister{}), changes,
		"pr-body", "--changes", "-", "--stencil-version", "v1.47.0")
	assert.NilError(t, err)
	assert.Assert(t, strings.Contains(stdout, "github.com/org/mod"), stdout)
	assert.Assert(t, strings.Contains(stdout, "https://github.com/org/mod/releases/tag/v1.1.0"), stdout)
	assert.Assert(t, strings.Contains(stdout, "<code>v1.47.0</code>"), stdout)
}

func TestStencilPRBodyCommandFile(t *testing.T) {
	dir := t.TempDir()
	changesPath := filepath.Join(dir, "changes.json")
	servicePath := filepath.Join(dir, "service.yaml")
	assert.NilError(t, os.WriteFile(changesPath, []byte(`[{"module":"github.com/org/mod","old":"v1.0.0","new":"v1.1.0-rc.1"}]`), 0o600))
	assert.NilError(t, os.WriteFile(servicePath, []byte("modules:\n  - name: github.com/org/mod\n    channel: rc\n"), 0o600))

	stdout, _, err := runSubcommand(t, newStencilCommandWithLister(fakeLister{}), "",
		"pr-body", "--changes", changesPath, "--stencil-version", "v1.47.0", "--service-yaml", servicePath)
	assert.NilError(t, err)
	assert.Assert(t, strings.Contains(stdout, "github.com/org/mod"), stdout)
}

func TestStencilPRBodyCommandNote(t *testing.T) {
	changes := `[{"module":"github.com/org/mod","old":"v1.0.0","new":"v1.1.0"}]`
	stdout, _, err := runSubcommand(t, newStencilCommandWithLister(fakeLister{}), changes,
		"pr-body", "--changes", "-", "--note", "Moving to a new vendor.")
	assert.NilError(t, err)
	assert.Assert(t, strings.HasPrefix(stdout, "Moving to a new vendor.\n\n---\n\n"), stdout)
}
