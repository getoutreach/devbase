// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Finds the release units of the old configuration for
// config convert: the Go submodules that release-submodules.sh tags.

package release

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/getoutreach/devbase/v2/internal/release/native"
	"github.com/getoutreach/devbase/v2/internal/release/releaserc"
	"github.com/getoutreach/devbase/v2/internal/release/spec"
	"go.yaml.in/yaml/v3"
)

// convertUnits adds the units of the old configuration to cfg and
// returns notes for the user. A folder with a go.mod is a Go module unit
// when an exec hook runs release-submodules.sh and the script names the
// folder. The unit mirrors the root version, or has its own version when
// the folder has a .releaserc.yaml for a nested semantic-release run.
// The units replace the hook, so cfg loses it. The old configuration
// does not release the other folders with a go.mod or a package.json,
// so they are only in a note.
func convertUnits(dir string, cfg *spec.Config) ([]string, error) {
	mods, pkgs, err := findPackages(dir)
	if err != nil {
		return nil, err
	}
	var notes []string
	if script := takeSubmodulesHook(cfg); script != "" {
		b, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(script)))
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", script, err)
		}
		rootMod, err := readGoMod(filepath.Join(dir, "go.mod"))
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return nil, err
		}
		var rest []string
		for _, m := range mods {
			if !namesDir(string(b), m) {
				rest = append(rest, m)
				continue
			}
			u, nested, err := convertGoUnit(dir, m, rootMod, cfg)
			if err != nil {
				return nil, err
			}
			cfg.Units = append(cfg.Units, u)
			if nested != "" {
				notes = append(notes, fmt.Sprintf("delete %s: the unit %s replaces it", nested, m))
			}
		}
		if len(cfg.Units) == 0 {
			return nil, fmt.Errorf("%w: %s names no folder with a go.mod; write the units key by hand",
				spec.ErrUnsupported, script)
		}
		notes = append(notes, fmt.Sprintf("delete %s: the units replace it", script))
		mods = rest
	}
	if len(mods) > 0 {
		notes = append(notes, fmt.Sprintf("the Go modules in %s are not units, because the old configuration "+
			"does not release them; add go-module units to release them", strings.Join(mods, ", ")))
	}
	// The npm publisher releases its package with the root version.
	for _, pl := range cfg.Plugins {
		if pl.NPM != nil {
			pkgs = slices.DeleteFunc(pkgs, func(p string) bool { return p == path.Clean(pl.NPM.PkgRoot) })
		}
	}
	if len(pkgs) > 0 {
		notes = append(notes, fmt.Sprintf("the npm packages in %s are not units, because the old configuration "+
			"does not release them; if other jobs release them, for example on a tag, add npm units",
			strings.Join(pkgs, ", ")))
	}
	return notes, nil
}

// findPackages returns the folders below dir, as slash paths, that have a
// go.mod or a package.json. Like the go command, it skips the folders
// whose names start with "." or "_", testdata and vendor, and it also
// skips node_modules.
func findPackages(dir string) (mods, pkgs []string, err error) {
	err = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		name := d.Name()
		switch {
		case rel == ".":
			return nil
		case d.IsDir():
			if strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") || slices.Contains(skipDirs, name) {
				return filepath.SkipDir
			}
		case filepath.Dir(rel) == ".":
			// The files of the root are the root release.
		case name == "go.mod":
			mods = append(mods, filepath.ToSlash(filepath.Dir(rel)))
		case name == "package.json":
			pkgs = append(pkgs, filepath.ToSlash(filepath.Dir(rel)))
		}
		return nil
	})
	if err != nil {
		return nil, nil, fmt.Errorf("find the Go modules and npm packages: %w", err)
	}
	return mods, pkgs, nil
}

// skipDirs are the folders that findPackages skips.
//
//nolint:gochecknoglobals // Why: read-only list.
var skipDirs = []string{"testdata", "vendor", "node_modules"}

// takeSubmodulesHook removes the exec hooks that only run
// release-submodules.sh, with at most one argument, from cfg, and returns
// the path of the script, or "". A hook that runs the script with other
// commands stays, so that the conversion fails.
func takeSubmodulesHook(cfg *spec.Config) string {
	script := ""
	cfg.Plugins = slices.DeleteFunc(cfg.Plugins, func(pl spec.Plugin) bool {
		h := pl.Exec
		if h == nil {
			return false
		}
		for _, cmd := range []*string{&h.GenerateNotes, &h.Prepare, &h.Publish, &h.Success} {
			fields := strings.Fields(*cmd)
			if len(fields) == 0 || len(fields) > 2 || path.Base(fields[0]) != native.SubmodulesScript ||
				!filepath.IsLocal(fields[0]) {
				continue
			}
			script, *cmd = path.Clean(fields[0]), ""
		}
		return *h == spec.ExecHooks{}
	})
	return script
}

// namesDir reports whether the script text names the folder dir as a
// whole path, such as "api" in "api/v$VERSION", but not in "openapi".
func namesDir(text, dir string) bool {
	return regexp.MustCompile(`(^|[^\w./-])` + regexp.QuoteMeta(dir) + `($|[^\w.-])`).MatchString(text)
}

// convertGoUnit returns the Go module unit of the folder m. A
// .releaserc.yaml in m gives the unit its own version and, when it is
// not the Go tag format, its tag format; convertGoUnit then returns the
// path of that file. The unit pins the root go.mod only when the old
// release commit has go.mod, as the script does.
func convertGoUnit(dir, m string, rootMod *goMod, cfg *spec.Config) (u spec.Unit, nested string, err error) {
	u = spec.Unit{Kind: spec.UnitGoModule, Dir: m}
	mod, err := readGoMod(filepath.Join(dir, filepath.FromSlash(m), "go.mod"))
	if err != nil {
		return u, "", err
	}
	if rootMod != nil && rootMod.requires[mod.module] && !(&Repo{Config: cfg}).commitsFile("go.mod") {
		u.Go = &spec.GoUnit{PinRoot: spec.GoPinNever}
	}
	nested = path.Join(m, releaserc.File)
	b, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(nested)))
	if errors.Is(err, fs.ErrNotExist) {
		return u, "", nil
	}
	if err != nil {
		return u, "", fmt.Errorf("read %s: %w", nested, err)
	}
	var rc struct {
		TagFormat string `yaml:"tagFormat"`
	}
	if err := yaml.Unmarshal(b, &rc); err != nil {
		return u, "", fmt.Errorf("parse %s: %w", nested, err)
	}
	u.Strategy = spec.StrategyIndependent
	if rc.TagFormat != "" {
		if prefix, err := spec.GoTagPrefix(m, mod.module); err != nil || rc.TagFormat != prefix+"v${version}" {
			u.TagFormat = spec.TagFormat(rc.TagFormat)
		}
	}
	return u, nested, nil
}
