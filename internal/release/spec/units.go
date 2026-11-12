// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Release units: parts of a repository, such as a Go
// submodule or an npm package, that release with their own tags.

package spec

import (
	"fmt"
	"path"
	"regexp"
	"strconv"
	"strings"

	"github.com/Masterminds/semver/v3"
)

// RootUnitName is the name of the root release in logs and the plan. A
// unit cannot have it.
const RootUnitName = "root"

// UnitKind selects the version rules and the publishers of a unit.
type UnitKind string

// These are the unit kinds.
const (
	// UnitGoModule is a nested Go module. Its tags are
	// "<module subdirectory>/v${version}", as Go requires, and the tag is
	// the release.
	UnitGoModule UnitKind = "go-module"

	// UnitNPM is an npm package, published with npm from its folder.
	UnitNPM UnitKind = "npm"

	// UnitGeneric is a part of the repository with its own tags and
	// notes, and an optional forge release.
	UnitGeneric UnitKind = "generic"
)

// UnitStrategy is how a unit gets its next version.
type UnitStrategy string

// These are the version strategies.
const (
	// StrategyIndependent computes the version from the commits of the
	// unit since its last tag, as the root release does.
	StrategyIndependent UnitStrategy = "independent"

	// StrategyMirror gives the unit the version of the root release, when
	// the root releases and the commits of the unit give a bump. It is
	// the default strategy for Go submodules.
	StrategyMirror UnitStrategy = "mirror"
)

// GoPin is how a release of a Go unit updates the root go.mod.
type GoPin string

// These are the GoPin values.
const (
	// GoPinAuto sets the root go.mod requirement of the module to the
	// new version in the release commit, when the root module requires
	// it.
	GoPinAuto GoPin = "auto"

	// GoPinNever leaves the root go.mod as it is.
	GoPinNever GoPin = "never"
)

// RootPaths are the path rules of the root release in a repository with
// units. Without them, the root release uses every commit, as
// semantic-release does.
type RootPaths struct {
	// Exclude are path globs (see MatchPath) whose changes do not count
	// for the root release, such as "node/**".
	Exclude []string

	// Shared are path globs of files that the root release and the
	// units share, such as ".circleci/config.yml". A change to them
	// counts for the root release only when the commit changes no unit.
	Shared []string
}

// IsZero reports whether r has no rules, so that the root release uses
// every commit.
func (r *RootPaths) IsZero() bool {
	return len(r.Exclude) == 0 && len(r.Shared) == 0
}

// Unit is an extra release unit: a part of the repository with its own
// tags, version, notes, git notes and publishers. The root release is not
// a Unit.
type Unit struct {
	// Name is a short ID, unique in the repository, for logs, the plan
	// and the --unit flag. Default: the base name of Dir.
	Name string

	// Kind selects the version rules and the publishers.
	Kind UnitKind

	// Dir is the folder of the unit, relative to the repository.
	Dir string

	// Paths are path globs of the files that belong to the unit.
	// Default: Dir. The folder of a nested unit never belongs to it.
	Paths []string

	// Exclude are path globs of files in Paths that do not belong to
	// the unit, such as a README.
	Exclude []string

	// TagFormat is the tag of a version. A Go module uses
	// "<module subdirectory>/v${version}", which is the default.
	TagFormat TagFormat

	// Strategy is how the unit gets its version. Default: mirror for a
	// Go module, independent for the other kinds.
	Strategy UnitStrategy

	// SkipPrereleases releases the unit only from the release branch.
	SkipPrereleases bool

	// AllowMajorVersions lets an independent unit release a new major
	// version when the root config blocks them (MajorCheck).
	AllowMajorVersions bool

	// ForgeRelease makes a forge release for each tag. It is never the
	// latest release of the repository.
	ForgeRelease bool

	// NPM is the npm publisher of an npm unit. Its PkgRoot defaults to
	// Dir.
	NPM *NPMPlugin

	// Go holds the options of a Go module unit.
	Go *GoUnit
}

// GoUnit holds the options of a Go module unit.
type GoUnit struct {
	// ModulePath is the module path in <Dir>/go.mod. The loader reads it.
	ModulePath string

	// PinRoot is how a release updates the root go.mod. Default:
	// GoPinAuto.
	PinRoot GoPin

	// RootRequires and RootReplaces report whether the root go.mod has a
	// require and a replace directive for ModulePath. The loader reads
	// them.
	RootRequires, RootReplaces bool
}

// SetDefaults fills in the defaults of the unset fields of u.
func (u *Unit) SetDefaults() {
	if u.Name == "" {
		u.Name = path.Base(u.Dir)
	}
	if len(u.Paths) == 0 && u.Dir != "" {
		u.Paths = []string{u.Dir}
	}
	switch u.Kind {
	case UnitGoModule:
		if u.Go == nil {
			u.Go = &GoUnit{}
		}
		if u.Go.PinRoot == "" {
			u.Go.PinRoot = GoPinAuto
		}
		if u.Strategy == "" {
			u.Strategy = StrategyMirror
		}
		if u.TagFormat == "" && u.Go.ModulePath != "" {
			if prefix, err := GoTagPrefix(u.Dir, u.Go.ModulePath); err == nil {
				u.TagFormat = TagFormat(prefix + "v" + versionVar)
			}
		}
	case UnitNPM:
		if u.NPM == nil {
			u.NPM = &NPMPlugin{}
		}
		if u.NPM.PkgRoot == "" {
			u.NPM.PkgRoot = u.Dir
		}
	case UnitGeneric:
	}
	if u.Strategy == "" {
		u.Strategy = StrategyIndependent
	}
}

// unitNameRegex matches a unit name: letters, digits, ".", "_", "-" and
// "/", starting with a letter or digit.
var unitNameRegex = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]*$`)

// validateUnits checks the units and the root path rules: unique names
// and tag formats, valid paths and globs, and the options of each kind.
func (c *Config) validateUnits() error {
	for _, g := range append(append([]string(nil), c.RootPaths.Exclude...), c.RootPaths.Shared...) {
		if err := ValidatePathGlob(g); err != nil {
			return fmt.Errorf("root path %q: %w", g, err)
		}
	}
	if len(c.Units) == 0 {
		if !c.RootPaths.IsZero() {
			return fmt.Errorf("%w: root path rules without units", ErrUnsupported)
		}
		return nil
	}
	names, formats := map[string]bool{}, map[TagFormat]string{RootTagFormat: RootUnitName}
	for i := range c.Units {
		u := &c.Units[i]
		if err := u.validate(c); err != nil {
			return fmt.Errorf("unit %q: %w", u.Name, err)
		}
		if names[u.Name] {
			return fmt.Errorf("%w: two units named %q", ErrUnsupported, u.Name)
		}
		names[u.Name] = true
		if other, ok := formats[u.TagFormat]; ok {
			return fmt.Errorf("%w: units %q and %q have the tag format %q", ErrUnsupported, other, u.Name, u.TagFormat)
		}
		formats[u.TagFormat] = u.Name
	}
	return nil
}

// validate checks one unit of c.
func (u *Unit) validate(c *Config) error {
	switch {
	case !unitNameRegex.MatchString(u.Name) || u.Name == RootUnitName:
		return fmt.Errorf("%w: invalid unit name %q", ErrUnsupported, u.Name)
	case !validDir(u.Dir):
		return fmt.Errorf("%w: invalid dir %q", ErrUnsupported, u.Dir)
	case len(u.Paths) == 0:
		return fmt.Errorf("%w: no paths", ErrUnsupported)
	}
	for _, g := range append(append([]string(nil), u.Paths...), u.Exclude...) {
		if err := ValidatePathGlob(g); err != nil {
			return fmt.Errorf("path %q: %w", g, err)
		}
	}
	if err := u.TagFormat.Validate(); err != nil {
		return err
	}
	if err := u.validateStrategy(c); err != nil {
		return err
	}
	if u.ForgeRelease && c.Forge() == nil {
		return fmt.Errorf("%w: a unit forge release needs the forge publisher of the root release", ErrUnsupported)
	}
	return u.validateKind()
}

// validateStrategy checks the version strategy and its options.
func (u *Unit) validateStrategy(c *Config) error {
	switch u.Strategy {
	case StrategyIndependent:
		return nil
	case StrategyMirror:
	default:
		return fmt.Errorf("%w: strategy %q", ErrUnsupported, u.Strategy)
	}
	switch {
	case u.Kind == UnitNPM:
		return fmt.Errorf("%w: an npm unit cannot use the mirror strategy", ErrUnsupported)
	case u.AllowMajorVersions:
		return fmt.Errorf("%w: allowMajorVersions is for independent units", ErrUnsupported)
	}
	// A mirror unit releases only with the root release. When the root
	// ignores its files, a change to them alone would never release.
	probe := u.Dir + "/go.mod"
	for _, g := range c.RootPaths.Exclude {
		if MatchPath(g, probe) {
			return fmt.Errorf("%w: the root path rule %q excludes the mirror unit", ErrUnsupported, g)
		}
	}
	return nil
}

// validateKind checks the options of the unit kind.
func (u *Unit) validateKind() error {
	switch u.Kind {
	case UnitGoModule:
		if u.NPM != nil {
			return fmt.Errorf("%w: a Go module unit has no npm publisher", ErrUnsupported)
		}
		return u.validateGo()
	case UnitNPM:
		if u.Go != nil {
			return fmt.Errorf("%w: an npm unit has no Go options", ErrUnsupported)
		}
		if u.NPM == nil || !validDir(u.NPM.PkgRoot) {
			return fmt.Errorf("%w: npm unit without a package folder", ErrUnsupported)
		}
		return nil
	case UnitGeneric:
		if u.NPM != nil || u.Go != nil {
			return fmt.Errorf("%w: a generic unit has no npm or Go options", ErrUnsupported)
		}
		return nil
	default:
		return fmt.Errorf("%w: unit kind %q", ErrUnsupported, u.Kind)
	}
}

// validateGo checks a Go module unit: the module path, the pin mode, and
// the tag format that Go requires.
func (u *Unit) validateGo() error {
	g := u.Go
	switch {
	case g == nil || g.ModulePath == "":
		return fmt.Errorf("%w: Go module unit without a module path", ErrUnsupported)
	case g.PinRoot != GoPinAuto && g.PinRoot != GoPinNever:
		return fmt.Errorf("%w: pinRoot %q", ErrUnsupported, g.PinRoot)
	}
	prefix, err := GoTagPrefix(u.Dir, g.ModulePath)
	if err != nil {
		return err
	}
	if want := TagFormat(prefix + "v" + versionVar); u.TagFormat != want {
		return fmt.Errorf("%w: the tag format of module %s must be %q, not %q",
			ErrUnsupported, g.ModulePath, want, u.TagFormat)
	}
	return nil
}

// validDir reports whether dir is a clean relative folder in the
// repository.
func validDir(dir string) bool {
	return dir != "." && isLocalPath(dir)
}

// majorSuffixRegex matches the major version suffix of a module path.
var majorSuffixRegex = regexp.MustCompile(`/v(\d+)$`)

// GoMajorSuffix returns the major version suffix of a module path, such
// as "/v2", or "".
func GoMajorSuffix(modulePath string) string {
	m := majorSuffixRegex.FindStringSubmatch(modulePath)
	if m == nil {
		return ""
	}
	if n, err := strconv.Atoi(m[1]); err != nil || n < 2 {
		return ""
	}
	return m[0]
}

// GoTagPrefix returns the tag prefix of the module at modulePath in the
// folder dir: the module subdirectory and "/". Go says that the module
// subdirectory is the part of the module path that names the folder,
// without the major version suffix. It fails when the module path does
// not end with dir, because the tag prefix is then not clear.
func GoTagPrefix(dir, modulePath string) (string, error) {
	suffix := GoMajorSuffix(modulePath)
	sub := dir
	// A major subdirectory, such as api/v2 for .../api/v2.
	if suffix != "" && strings.HasSuffix(dir, suffix) {
		sub = strings.TrimSuffix(dir, suffix)
	}
	if !strings.HasSuffix(strings.TrimSuffix(modulePath, suffix), "/"+sub) {
		return "", fmt.Errorf("%w: module path %s does not end with its folder %s", ErrUnsupported, modulePath, dir)
	}
	return sub + "/", nil
}

// CheckGoMajor checks that a Go module can have version: from v2 on, the
// module path must end with the matching /vN suffix, and before v2 it
// must have none.
func CheckGoMajor(modulePath string, version *semver.Version) error {
	want := ""
	if version.Major() >= 2 { //nolint:mnd // Why: Go starts major version suffixes at v2.
		want = "/v" + strconv.FormatUint(version.Major(), 10)
	}
	if got := GoMajorSuffix(modulePath); got != want {
		return fmt.Errorf("%w: version %s of module %s needs the module path suffix %q",
			ErrUnsupported, version, modulePath, want)
	}
	return nil
}
