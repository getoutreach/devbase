// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: The root and units keys of the release section: the
// release units of the repository and the path rules of the root
// release.

package native

import (
	"github.com/getoutreach/devbase/v2/internal/release/spec"
)

// rootPaths are the path rules of the root release (spec.RootPaths).
type rootPaths struct {
	Exclude []string `yaml:"exclude,omitempty"`
	Shared  []string `yaml:"shared,omitempty"`
}

// unit is a release unit (spec.Unit). The loader of the repository
// fills in the defaults, because some of them come from the files of the
// repository, such as the module path in go.mod.
type unit struct {
	Name               string   `yaml:"name,omitempty"`
	Kind               string   `yaml:"kind"`
	Dir                string   `yaml:"dir"`
	Paths              []string `yaml:"paths,omitempty"`
	Exclude            []string `yaml:"exclude,omitempty"`
	TagFormat          string   `yaml:"tagFormat,omitempty"`
	Strategy           string   `yaml:"strategy,omitempty"`
	SkipPrereleases    bool     `yaml:"skipPrereleases,omitempty"`
	AllowMajorVersions bool     `yaml:"allowMajorVersions,omitempty"`
	ForgeRelease       bool     `yaml:"forgeRelease,omitempty"`
	NPM                *unitNPM `yaml:"npm,omitempty"`
	Go                 *unitGo  `yaml:"go,omitempty"`
}

// unitNPM is the npm publisher of an npm unit.
type unitNPM struct {
	PackageRoot string `yaml:"packageRoot,omitempty"`
	Install     string `yaml:"install,omitempty"`
}

// unitGo holds the options of a Go module unit.
type unitGo struct {
	PinRoot string `yaml:"pinRoot,omitempty"`
}

// setUnits adds the units and the root path rules of the section to
// cfg, as the section has them.
func (s *section) setUnits(cfg *spec.Config) {
	if s.Root != nil {
		cfg.RootPaths = spec.RootPaths{Exclude: emptyToNil(s.Root.Exclude), Shared: emptyToNil(s.Root.Shared)}
	}
	for i := range s.Units {
		u := &s.Units[i]
		out := spec.Unit{
			Name: u.Name, Kind: spec.UnitKind(u.Kind), Dir: u.Dir,
			Paths: emptyToNil(u.Paths), Exclude: emptyToNil(u.Exclude),
			TagFormat: spec.TagFormat(u.TagFormat), Strategy: spec.UnitStrategy(u.Strategy),
			SkipPrereleases: u.SkipPrereleases, AllowMajorVersions: u.AllowMajorVersions, ForgeRelease: u.ForgeRelease,
		}
		if u.NPM != nil {
			out.NPM = &spec.NPMPlugin{PkgRoot: u.NPM.PackageRoot, Install: u.NPM.Install}
		}
		if u.Go != nil {
			out.Go = &spec.GoUnit{PinRoot: spec.GoPin(u.Go.PinRoot)}
		}
		cfg.Units = append(cfg.Units, out)
	}
}

// fromUnits returns the root and units keys of cfg.
func fromUnits(cfg *spec.Config) (*rootPaths, []unit) {
	var root *rootPaths
	if !cfg.RootPaths.IsZero() {
		root = &rootPaths{Exclude: cfg.RootPaths.Exclude, Shared: cfg.RootPaths.Shared}
	}
	units := make([]unit, 0, len(cfg.Units))
	for i := range cfg.Units {
		u := &cfg.Units[i]
		out := unit{
			Name: u.Name, Kind: string(u.Kind), Dir: u.Dir, Paths: u.Paths, Exclude: u.Exclude,
			TagFormat: string(u.TagFormat), Strategy: string(u.Strategy),
			SkipPrereleases: u.SkipPrereleases, AllowMajorVersions: u.AllowMajorVersions, ForgeRelease: u.ForgeRelease,
		}
		if u.NPM != nil {
			out.NPM = &unitNPM{PackageRoot: u.NPM.PkgRoot, Install: u.NPM.Install}
		}
		if u.Go != nil {
			out.Go = &unitGo{PinRoot: string(u.Go.PinRoot)}
		}
		units = append(units, out)
	}
	return root, units
}
