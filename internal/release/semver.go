// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Version bumps with the node-semver rules that
// semantic-release depends on. Parsing and comparison use
// Masterminds/semver.

package release

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/Masterminds/semver/v3"
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

// bumpOrder lists the bumps from smallest to largest.
//
//nolint:gochecknoglobals // Why: read-only lookup table.
var bumpOrder = []Bump{BumpNone, BumpPatch, BumpMinor, BumpMajor}

// rank orders bumps so that the larger one can be chosen. An unknown
// bump has rank -1.
func (r Bump) rank() int {
	return slices.Index(bumpOrder, r)
}

// maxBump returns the larger of two bumps.
func maxBump(a, b Bump) Bump {
	if b.rank() > a.rank() {
		return b
	}
	return a
}

// ParseVersion parses a strict semantic version with an optional "v"
// prefix, as node-semver's parse does without the loose option.
func ParseVersion(s string) (*semver.Version, error) {
	v, err := semver.StrictNewVersion(strings.TrimPrefix(s, "v"))
	if err != nil {
		return nil, fmt.Errorf("%w: %q: %w", ErrInvalidVersion, s, err)
	}
	return v, nil
}

// isCleanVersion reports whether node-semver's semver.valid(semver.clean(s))
// accepts s: clean trims whitespace and any leading "=" and "v"
// characters before it parses.
func isCleanVersion(s string) bool {
	_, err := semver.StrictNewVersion(strings.TrimLeft(jsTrim(s), "=v"))
	return err == nil
}

// bumpVersion returns v bumped by t, with node-semver rules: a
// prerelease of the target version is promoted instead of bumped again
// (for example, a minor bump of 1.1.0-rc.2 is 1.1.0).
func bumpVersion(v *semver.Version, t Bump) *semver.Version {
	major, minor, patch := v.Major(), v.Minor(), v.Patch()
	pre := v.Prerelease() != ""
	switch t {
	case BumpMajor:
		if !pre || minor != 0 || patch != 0 {
			major++
		}
		minor, patch = 0, 0
	case BumpMinor:
		if !pre || patch != 0 {
			minor++
		}
		patch = 0
	case BumpPatch:
		if !pre {
			patch++
		}
	case BumpNone:
		return v
	}
	return semver.New(major, minor, patch, "", "")
}

// bumpPrerelease returns a prerelease version with its last numeric
// identifier incremented, or with ".0" added if it has none, as
// node-semver's inc(v, "prerelease") does.
func bumpPrerelease(v *semver.Version) *semver.Version {
	ids := prereleaseIDs(v)
	for i := len(ids) - 1; i >= 0; i-- {
		if n, err := strconv.ParseUint(ids[i], 10, 64); err == nil {
			ids[i] = strconv.FormatUint(n+1, 10)
			return withPrerelease(v, strings.Join(ids, "."))
		}
	}
	return withPrerelease(v, strings.Join(append(ids, "0"), "."))
}

// withPrerelease returns the version core of v with prerelease pre.
func withPrerelease(v *semver.Version, pre string) *semver.Version {
	return semver.New(v.Major(), v.Minor(), v.Patch(), pre, "")
}

// hasPrereleaseID reports whether one of the prerelease identifiers of
// v is id. releaserc rejects a numeric id, which node-semver would store
// as a number that the string comparison in semantic-release never
// matches.
func hasPrereleaseID(v *semver.Version, id string) bool {
	return slices.Contains(prereleaseIDs(v), id)
}

// prereleaseIDs returns the prerelease identifiers of v, or nil.
func prereleaseIDs(v *semver.Version) []string {
	if v.Prerelease() == "" {
		return nil
	}
	return strings.Split(v.Prerelease(), ".")
}
