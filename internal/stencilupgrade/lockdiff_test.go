// Copyright 2026 Outreach Corporation. Licensed under the Apache License 2.0.

// Description: Tests for the stencil lockfile diff.

package stencilupgrade_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/getoutreach/devbase/v2/internal/stencilupgrade"
	"github.com/getoutreach/stencil/pkg/stencil"
	"gotest.tools/v3/assert"
)

// lock builds a Lockfile from alternating module name and version pairs.
func lock(pairs ...string) *stencil.Lockfile {
	l := &stencil.Lockfile{Version: "v1.47.0"}
	for i := 0; i < len(pairs); i += 2 {
		l.Modules = append(l.Modules, &stencil.LockfileModuleEntry{Name: pairs[i], Version: pairs[i+1]})
	}
	return l
}

func TestLockDiff(t *testing.T) {
	tests := []struct {
		name    string
		oldLock *stencil.Lockfile
		newLock *stencil.Lockfile
		want    []stencilupgrade.ModuleChange
	}{
		{
			name:    "upgraded",
			oldLock: lock("a", "v1.0.0"),
			newLock: lock("a", "v1.1.0"),
			want:    []stencilupgrade.ModuleChange{{Module: "a", Old: "v1.0.0", New: "v1.1.0"}},
		},
		{
			name:    "unchanged is omitted",
			oldLock: lock("a", "v1.0.0"),
			newLock: lock("a", "v1.0.0"),
		},
		{
			name:    "new module has empty old version",
			oldLock: lock("a", "v1.0.0"),
			newLock: lock("a", "v1.0.0", "b", "v0.1.0"),
			want:    []stencilupgrade.ModuleChange{{Module: "b", Old: "", New: "v0.1.0"}},
		},
		{
			name:    "removed module is omitted",
			oldLock: lock("a", "v1.0.0", "b", "v0.1.0"),
			newLock: lock("a", "v1.0.0"),
		},
		{
			name:    "unstable to unstable is included",
			oldLock: lock("a", "unstable"),
			newLock: lock("a", "unstable"),
			want: []stencilupgrade.ModuleChange{
				{Module: "a", Old: "unstable", New: "unstable", Unstable: true},
			},
		},
		{
			name:    "local to a release is included",
			oldLock: lock("a", "local"),
			newLock: lock("a", "v1.2.0"),
			want:    []stencilupgrade.ModuleChange{{Module: "a", Old: "local", New: "v1.2.0"}},
		},
		{
			name:    "result is sorted by module",
			oldLock: lock("c", "v1.0.0", "a", "v1.0.0"),
			newLock: lock("c", "v1.1.0", "a", "v1.1.0", "b", "v1.0.0"),
			want: []stencilupgrade.ModuleChange{
				{Module: "a", Old: "v1.0.0", New: "v1.1.0"},
				{Module: "b", Old: "", New: "v1.0.0"},
				{Module: "c", Old: "v1.0.0", New: "v1.1.0"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stencilupgrade.LockDiff(tt.oldLock, tt.newLock)
			assert.DeepEqual(t, got, tt.want)
		})
	}
}

func TestLoadLock(t *testing.T) {
	path := filepath.Join(t.TempDir(), "stencil.lock")
	content := "version: v1.47.0\nmodules:\n" +
		"  - name: github.com/getoutreach/stencil-golang\n" +
		"    url: https://github.com/getoutreach/stencil-golang\n" +
		"    version: v1.2.3\n" +
		"files: []\n"
	assert.NilError(t, os.WriteFile(path, []byte(content), 0o600))

	got, err := stencilupgrade.LoadLock(path)
	assert.NilError(t, err)
	assert.Equal(t, len(got.Modules), 1)
	assert.Equal(t, got.Modules[0].Name, "github.com/getoutreach/stencil-golang")
	assert.Equal(t, got.Modules[0].Version, "v1.2.3")
}

func TestLoadLockNoModules(t *testing.T) {
	path := filepath.Join(t.TempDir(), "stencil.lock")
	assert.NilError(t, os.WriteFile(path, []byte("version: v1.47.0\nmodules: []\n"), 0o600))

	_, err := stencilupgrade.LoadLock(path)
	assert.ErrorContains(t, err, "no modules")
}

func TestLoadLockMissingFile(t *testing.T) {
	_, err := stencilupgrade.LoadLock(filepath.Join(t.TempDir(), "missing.lock"))
	assert.Assert(t, err != nil)
}
