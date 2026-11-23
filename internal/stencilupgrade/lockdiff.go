// Copyright 2026 Outreach Corporation. Licensed under the Apache License 2.0.

// Description: Compares two stencil lockfiles to find changed modules.

package stencilupgrade

import (
	"cmp"
	"errors"
	"fmt"
	"os"
	"slices"

	"github.com/getoutreach/stencil/pkg/stencil"
	"go.yaml.in/yaml/v3"
)

// UnstableVersion is the version stencil records for a module that
// tracks a branch instead of a release. The name does not change when
// the module content does, so such modules are always reported.
const UnstableVersion = "unstable"

// errNoModules is returned when a lockfile decodes without any modules.
// A valid stencil.lock always lists at least one module, so an empty
// list means the file format changed.
var errNoModules = errors.New("lockfile has no modules")

// ModuleChange describes one module whose version changed between two
// lockfiles.
type ModuleChange struct {
	// Module is the module name.
	Module string `json:"module"`

	// Old is the version in the old lockfile. It is empty for a module
	// that is new.
	Old string `json:"old"`

	// New is the version in the new lockfile.
	New string `json:"new"`

	// Unstable is true when New is UnstableVersion.
	Unstable bool `json:"unstable,omitempty"`
}

// LoadLock decodes the stencil lockfile at path. It returns an error if
// the file cannot be read or decoded, or if it has no modules.
func LoadLock(path string) (*stencil.Lockfile, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read lockfile: %w", err)
	}

	var l stencil.Lockfile
	if err := yaml.Unmarshal(b, &l); err != nil {
		return nil, fmt.Errorf("decode lockfile %s: %w", path, err)
	}
	if len(l.Modules) == 0 {
		return nil, fmt.Errorf("%s: %w", path, errNoModules)
	}
	return &l, nil
}

// LockDiff returns the modules in newLock whose version differs from
// oldLock, sorted by module name. Modules that are unstable in newLock
// are always included. Modules removed from newLock are never included.
func LockDiff(oldLock, newLock *stencil.Lockfile) []ModuleChange {
	oldVersions := make(map[string]string, len(oldLock.Modules))
	for _, m := range oldLock.Modules {
		oldVersions[m.Name] = m.Version
	}

	var changes []ModuleChange
	for _, m := range newLock.Modules {
		old := oldVersions[m.Name]
		unstable := m.Version == UnstableVersion
		if old == m.Version && !unstable {
			continue
		}
		changes = append(changes, ModuleChange{Module: m.Name, Old: old, New: m.Version, Unstable: unstable})
	}

	slices.SortFunc(changes, func(a, b ModuleChange) int { return cmp.Compare(a.Module, b.Module) })
	return changes
}
