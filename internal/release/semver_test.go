// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Tests for version bumps.

package release

import (
	"testing"

	"github.com/Masterminds/semver/v3"
	"github.com/getoutreach/devbase/v2/internal/release/spec"
	"gotest.tools/v3/assert"
)

func mustVersion(t *testing.T, s string) *semver.Version {
	t.Helper()
	v, err := ParseVersion(s)
	assert.NilError(t, err)
	return v
}

func TestParseVersionRejectsInvalid(t *testing.T) {
	for _, s := range []string{"1.2", "1.2.3-", "01.2.3", "1.2.3-01", "v1.x.0", "1.2.3-rc..1"} {
		_, err := ParseVersion(s)
		assert.ErrorIs(t, err, ErrInvalidVersion, s)
	}
}

func TestParseVersionKeepsBuildMetadata(t *testing.T) {
	v := mustVersion(t, "v0.0.0-unstable+abc1234")
	assert.Equal(t, v.String(), "0.0.0-unstable+abc1234")
	assert.Equal(t, v.Prerelease(), "unstable")
	assert.Equal(t, v.Metadata(), "abc1234")
}

func TestBumpVersion(t *testing.T) {
	// The expected values follow node-semver inc().
	tests := []struct {
		in   string
		t    spec.Bump
		want string
	}{
		{"1.2.3", spec.BumpPatch, "1.2.4"},
		{"1.2.3", spec.BumpMinor, "1.3.0"},
		{"1.2.3", spec.BumpMajor, "2.0.0"},
		{"1.1.0-rc.2", spec.BumpMinor, "1.1.0"},
		{"1.1.0-rc.2", spec.BumpMajor, "2.0.0"},
		{"1.1.0-rc.2", spec.BumpPatch, "1.1.0"},
		{"1.1.1-rc.1", spec.BumpMinor, "1.2.0"},
		{"2.0.0-rc.1", spec.BumpMajor, "2.0.0"},
	}
	for _, tt := range tests {
		assert.Equal(t, bumpVersion(mustVersion(t, tt.in), tt.t).String(), tt.want, "%s %s", tt.in, tt.t)
	}
}

func TestBumpPrerelease(t *testing.T) {
	assert.Equal(t, bumpPrerelease(mustVersion(t, "1.1.0-rc.2")).String(), "1.1.0-rc.3")
	assert.Equal(t, bumpPrerelease(mustVersion(t, "1.1.0-rc.9")).String(), "1.1.0-rc.10")
	assert.Equal(t, bumpPrerelease(mustVersion(t, "1.1.0-rc")).String(), "1.1.0-rc.0")
	assert.Equal(t, bumpPrerelease(mustVersion(t, "1.1.0")).String(), "1.1.0-0")
}

func TestHasPrereleaseID(t *testing.T) {
	assert.Assert(t, hasPrereleaseID(mustVersion(t, "1.0.0-rc.1"), "rc"))
	assert.Assert(t, !hasPrereleaseID(mustVersion(t, "1.0.0-rc2.1"), "rc"))
	assert.Assert(t, !hasPrereleaseID(mustVersion(t, "1.0.0"), "rc"))
}
