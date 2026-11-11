// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Semantic version parsing, comparison and increments
// that match the node-semver behavior semantic-release depends on.

package release

import (
	"cmp"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
)

// ErrInvalidVersion is returned when a string is not a valid semantic version.
var ErrInvalidVersion = errors.New("invalid semantic version")

// Bump is the size of a version bump: the release type.
type Bump string

// These are the bumps, in increasing order of size.
const (
	// BumpNone means that no release is needed.
	BumpNone Bump = ""

	// BumpPatch is a patch release.
	BumpPatch Bump = "patch"

	// BumpMinor is a minor release.
	BumpMinor Bump = "minor"

	// BumpMajor is a major release.
	BumpMajor Bump = "major"
)

// bumpOrder lists the release types from smallest to largest.
//
//nolint:gochecknoglobals // Why: read-only lookup table.
var bumpOrder = []Bump{BumpNone, BumpPatch, BumpMinor, BumpMajor}

// rank orders release types so that the larger one can be chosen. An
// unknown type has rank -1.
func (r Bump) rank() int {
	return slices.Index(bumpOrder, r)
}

// maxBump returns the larger of two release types.
func maxBump(a, b Bump) Bump {
	if b.rank() > a.rank() {
		return b
	}
	return a
}

// Version is a semantic version. Build metadata is kept but does not
// affect precedence.
type Version struct {
	Major, Minor, Patch uint64

	// Prerelease holds the dot-separated prerelease identifiers.
	Prerelease []string

	// Build holds the build metadata, without the "+".
	Build string
}

// ParseVersion parses a semantic version with an optional "v" prefix.
func ParseVersion(s string) (Version, error) {
	invalid := fmt.Errorf("%w: %q", ErrInvalidVersion, s)
	var v Version
	rest, build, _ := strings.Cut(strings.TrimPrefix(s, "v"), "+")
	v.Build = build
	rest, pre, hasPre := strings.Cut(rest, "-")
	if hasPre {
		v.Prerelease = strings.Split(pre, ".")
		for _, id := range v.Prerelease {
			if id == "" || (isNumeric(id) && len(id) > 1 && id[0] == '0') {
				return Version{}, invalid
			}
		}
	}

	parts := strings.Split(rest, ".")
	if len(parts) != 3 {
		return Version{}, invalid
	}
	nums := [3]*uint64{&v.Major, &v.Minor, &v.Patch}
	for i, p := range parts {
		if len(p) > 1 && p[0] == '0' {
			return Version{}, invalid
		}
		n, err := strconv.ParseUint(p, 10, 64)
		if err != nil {
			return Version{}, invalid
		}
		*nums[i] = n
	}
	return v, nil
}

// String returns the version without a "v" prefix.
func (v Version) String() string {
	s := fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
	if len(v.Prerelease) > 0 {
		s += "-" + strings.Join(v.Prerelease, ".")
	}
	if v.Build != "" {
		s += "+" + v.Build
	}
	return s
}

// IsPrerelease reports whether the version has prerelease identifiers.
func (v Version) IsPrerelease() bool {
	return len(v.Prerelease) > 0
}

// Compare returns -1, 0 or 1 when v is lower than, equal to or higher
// than o, by semver precedence.
func (v Version) Compare(o Version) int {
	if c := cmp.Or(cmp.Compare(v.Major, o.Major), cmp.Compare(v.Minor, o.Minor), cmp.Compare(v.Patch, o.Patch)); c != 0 {
		return c
	}
	switch {
	case !v.IsPrerelease() && !o.IsPrerelease():
		return 0
	case !v.IsPrerelease():
		return 1
	case !o.IsPrerelease():
		return -1
	}
	for i := range min(len(v.Prerelease), len(o.Prerelease)) {
		if c := comparePrereleaseID(v.Prerelease[i], o.Prerelease[i]); c != 0 {
			return c
		}
	}
	return cmp.Compare(len(v.Prerelease), len(o.Prerelease))
}

// Inc returns the version bumped by t, with node-semver rules: a
// prerelease of the target version is promoted instead of bumped again
// (for example, a minor bump of 1.1.0-rc.2 is 1.1.0).
func (v Version) Inc(t Bump) Version {
	n := v.core()
	pre := v.IsPrerelease()
	switch t {
	case BumpMajor:
		if !pre || v.Minor != 0 || v.Patch != 0 {
			n.Major++
		}
		n.Minor, n.Patch = 0, 0
	case BumpMinor:
		if !pre || v.Patch != 0 {
			n.Minor++
		}
		n.Patch = 0
	case BumpPatch:
		if !pre {
			n.Patch++
		}
	case BumpNone:
		return v
	}
	return n
}

// IncPrerelease returns a prerelease version with its last numeric
// identifier incremented, or with ".0" added if it has none, as
// node-semver's inc(v, "prerelease") does.
func (v Version) IncPrerelease() Version {
	n := v.WithPrerelease(slices.Clone(v.Prerelease)...)
	for i := len(n.Prerelease) - 1; i >= 0; i-- {
		if num, err := strconv.ParseUint(n.Prerelease[i], 10, 64); err == nil {
			n.Prerelease[i] = strconv.FormatUint(num+1, 10)
			return n
		}
	}
	n.Prerelease = append(n.Prerelease, "0")
	return n
}

// WithPrerelease returns the version core with the given prerelease
// identifiers.
func (v Version) WithPrerelease(ids ...string) Version {
	n := v.core()
	n.Prerelease = ids
	return n
}

// core returns the major, minor and patch numbers only.
func (v Version) core() Version {
	return Version{Major: v.Major, Minor: v.Minor, Patch: v.Patch}
}

// comparePrereleaseID compares two prerelease identifiers: numeric
// identifiers compare as numbers and sort before alphanumeric ones.
func comparePrereleaseID(a, b string) int {
	ai, aErr := strconv.ParseUint(a, 10, 64)
	bi, bErr := strconv.ParseUint(b, 10, 64)
	switch {
	case aErr == nil && bErr == nil:
		return cmp.Compare(ai, bi)
	case aErr == nil:
		return -1
	case bErr == nil:
		return 1
	}
	return strings.Compare(a, b)
}

// isNumeric reports whether s is a non-empty string of ASCII digits.
func isNumeric(s string) bool {
	return s != "" && strings.Trim(s, "0123456789") == ""
}
