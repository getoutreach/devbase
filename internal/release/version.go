// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Finds the last release on a branch and computes the
// next version, with the same rules as semantic-release 25.

package release

import (
	"errors"
	"fmt"
	"slices"

	"github.com/Masterminds/semver/v3"
)

// ErrMajorVersion is returned when the next version is a new major
// version but the configuration blocks major versions.
var ErrMajorVersion = errors.New("major version bump is not allowed")

// LastRelease returns the highest tag that counts as a release for
// branch b, or nil. On a release branch, prerelease tags never count.
// On a prerelease branch, a prerelease tag counts only when it is on
// the branch channel and has the branch prerelease identifier.
func LastRelease(b *Branch, tags []Tag) *Tag {
	var best *Tag
	for i := range tags {
		t := &tags[i]
		ok := t.Version.Prerelease() == ""
		if b.IsPrerelease() && !ok {
			ok = slices.Contains(t.Channels, b.Channel) && hasPrereleaseID(t.Version, b.Prerelease)
		}
		if ok && (best == nil || t.Version.GreaterThan(best.Version)) {
			best = t
		}
	}
	return best
}

// NextVersion returns the version for a release of type t on branch b.
// tags are all release tags merged into the branch.
func NextVersion(b *Branch, last *Tag, tags []Tag, t Bump) *semver.Version {
	if !b.IsPrerelease() {
		if last == nil {
			return semver.New(1, 0, 0, "", "")
		}
		return bumpVersion(last.Version, t)
	}
	firstPre := b.Prerelease + ".1"
	if last == nil {
		return semver.New(1, 0, 0, firstPre, "")
	}

	if last.Version.Prerelease() != "" && slices.Contains(last.Channels, b.Channel) {
		next := bumpPrerelease(last.Version)
		highest := slices.MaxFunc(tags, func(x, y Tag) int { return x.Version.Compare(y.Version) }).Version
		if bumped := withPrerelease(bumpVersion(highest, t), firstPre); bumped.GreaterThan(next) {
			return bumped
		}
		return next
	}
	core := withPrerelease(last.Version, "")
	return withPrerelease(bumpVersion(core, t), firstPre)
}

// CheckMajor returns ErrMajorVersion when next has a higher major
// version than last. It replaces major-release-checker.js.
func CheckMajor(last *Tag, next *semver.Version) error {
	lastMajor := uint64(1)
	if last != nil {
		lastMajor = last.Version.Major()
	}
	if next.Major() != lastMajor {
		return fmt.Errorf("%w: %d to %d", ErrMajorVersion, lastMajor, next.Major())
	}
	return nil
}
