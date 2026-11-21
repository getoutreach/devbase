// Copyright 2026 Outreach Corporation. Licensed under the Apache License 2.0.

// Description: End-to-end tests for the "stencil" CLI subcommands.

package main

import (
	"os"
	"path/filepath"
	"testing"

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
