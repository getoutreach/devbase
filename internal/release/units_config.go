// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Adds the release units to a repository's release model:
// defaults, the go.mod and package.json checks, and the digest.

package release

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/getoutreach/devbase/v2/internal/release/releaserc"
	"github.com/getoutreach/devbase/v2/internal/release/spec"
)

// ErrUnitConfig is returned for a unit config that the files of the
// repository do not support.
var ErrUnitConfig = errors.New("invalid release unit")

// SetUnits sets the release units and the root path rules of r, as the
// loader of a config with units does: it reads the module paths of the Go
// units and the root go.mod, fills in the defaults, checks the model and
// the files, and adds the units to the digest, so that a plan made with
// other units does not publish.
func (r *Repo) SetUnits(units []spec.Unit, root spec.RootPaths) error {
	dir := r.Git.Dir
	rootMod, err := readGoMod(filepath.Join(dir, "go.mod"))
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	units = slices.Clone(units)
	for i := range units {
		u := &units[i]
		u.Go, u.NPM = clonePtr(u.Go), clonePtr(u.NPM)
		if u.Kind == spec.UnitGoModule {
			if err := resolveGoUnit(dir, u, rootMod); err != nil {
				return err
			}
		}
		u.SetDefaults()
	}
	r.Config.Units, r.Config.RootPaths = units, root
	warnings, err := r.Config.Validate()
	if err != nil {
		return err
	}
	r.Warnings = append(r.Warnings, warnings...)
	for i := range units {
		if err := r.checkUnitFiles(&units[i]); err != nil {
			return fmt.Errorf("unit %q: %w", units[i].Name, err)
		}
	}
	// The digest needs only a stable encoding of the model, not a file
	// format, so the field names of the spec types are enough.
	//nolint:musttag // Why: the spec types are not a file format.
	b, err := json.Marshal(struct {
		Units []spec.Unit    `json:"units"`
		Root  spec.RootPaths `json:"root"`
	}{units, root})
	if err != nil {
		return fmt.Errorf("encode the units: %w", err)
	}
	h := sha256.Sum256(append([]byte(r.Digest+"\x00units\x00"), b...))
	r.Digest = "sha256:" + hex.EncodeToString(h[:])
	return nil
}

// clonePtr returns a copy of *p, or nil.
func clonePtr[T any](p *T) *T {
	if p == nil {
		return nil
	}
	c := *p
	return &c
}

// resolveGoUnit reads the module path of a Go unit from its go.mod, and
// whether the root go.mod requires and replaces it.
func resolveGoUnit(dir string, u *spec.Unit, rootMod *goMod) error {
	if u.Go == nil {
		u.Go = &spec.GoUnit{}
	}
	if u.Go.ModulePath == "" {
		mod, err := readGoMod(filepath.Join(dir, filepath.FromSlash(u.Dir), "go.mod"))
		if err != nil {
			return fmt.Errorf("%w %q: %w", ErrUnitConfig, u.Name, err)
		}
		u.Go.ModulePath = mod.module
	}
	if rootMod != nil {
		u.Go.RootRequires = rootMod.requires[u.Go.ModulePath]
		u.Go.RootReplaces = rootMod.replaces[u.Go.ModulePath]
	}
	return nil
}

// checkUnitFiles checks a unit against the files of the repository: an
// npm unit has a package.json; a Go unit that pins the root go.mod has a
// release commit with go.mod; and no nested .releaserc.yaml also
// releases the unit.
func (r *Repo) checkUnitFiles(u *spec.Unit) error {
	dir := r.Git.Dir
	if u.NPM != nil {
		if _, err := readNPMPackage(filepath.Join(dir, filepath.FromSlash(u.NPM.PkgRoot))); err != nil {
			return err
		}
	}
	if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(u.Dir), releaserc.File)); err == nil {
		return fmt.Errorf("%w: %s/%s also releases the unit; remove it", ErrUnitConfig, u.Dir, releaserc.File)
	}
	if g := u.Go; g != nil && g.PinRoot == spec.GoPinAuto && g.RootRequires && !r.commitsFile("go.mod") {
		return fmt.Errorf("%w: the root go.mod requires the module, so its release needs a release commit "+
			"with go.mod and go.sum (add them to the commit publisher, or set pinRoot: never)", ErrUnitConfig)
	}
	return nil
}

// commitsFile reports whether a release commit plugin commits file.
func (r *Repo) commitsFile(file string) bool {
	for _, pl := range r.Config.Plugins {
		if c := pl.Commit; c != nil {
			globs := c.Files
			if globs == nil {
				globs = defaultCommitFiles
			}
			if slices.ContainsFunc(globs, func(g string) bool { return matchCommitGlob(g, file) }) {
				return true
			}
		}
	}
	return false
}

// goMod is the part of a go.mod file that units read.
type goMod struct {
	// module is the module path.
	module string

	// requires and replaces are the module paths of the require and
	// replace directives.
	requires, replaces map[string]bool
}

// readGoMod reads the module, require and replace directives of a go.mod
// file. It reads only the first token of each directive line, which is
// the module path.
func readGoMod(file string) (*goMod, error) {
	b, err := os.ReadFile(file)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", file, err)
	}
	m := &goMod{requires: map[string]bool{}, replaces: map[string]bool{}}
	block := ""
	for line := range strings.SplitSeq(string(b), "\n") {
		if i := strings.Index(line, "//"); i >= 0 {
			line = line[:i]
		}
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		verb := block
		switch {
		case block != "" && fields[0] == ")":
			block = ""
			continue
		case block == "" && len(fields) == 2 && fields[1] == "(":
			block = fields[0]
			continue
		case block == "":
			verb, fields = fields[0], fields[1:]
		}
		if len(fields) == 0 {
			continue
		}
		name := strings.Trim(fields[0], `"`)
		switch verb {
		case "module":
			m.module = name
		case "require":
			m.requires[name] = true
		case "replace":
			m.replaces[name] = true
		}
	}
	if m.module == "" {
		return nil, fmt.Errorf("%w: %s has no module directive", ErrUnitConfig, file)
	}
	return m, nil
}
