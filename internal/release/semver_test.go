// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Tests for semantic version handling.

package release

import (
	"testing"

	"gotest.tools/v3/assert"
)

func mustVersion(t *testing.T, s string) Version {
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
	assert.DeepEqual(t, v.Prerelease, []string{"unstable"})
}

func TestCompare(t *testing.T) {
	ordered := []string{"1.0.0-rc.1", "1.0.0-rc.2", "1.0.0-rc.10", "1.0.0-rc.a", "1.0.0", "1.0.1", "1.1.0-rc.1", "1.1.0", "2.0.0"}
	for i := 0; i+1 < len(ordered); i++ {
		a, b := mustVersion(t, ordered[i]), mustVersion(t, ordered[i+1])
		assert.Equal(t, a.Compare(b), -1, "%s < %s", ordered[i], ordered[i+1])
		assert.Equal(t, b.Compare(a), 1, "%s > %s", ordered[i+1], ordered[i])
		assert.Equal(t, a.Compare(mustVersion(t, ordered[i])), 0)
	}
}

func TestInc(t *testing.T) {
	// The expected values follow node-semver inc().
	tests := []struct {
		in   string
		t    Bump
		want string
	}{
		{"1.2.3", BumpPatch, "1.2.4"},
		{"1.2.3", BumpMinor, "1.3.0"},
		{"1.2.3", BumpMajor, "2.0.0"},
		{"1.1.0-rc.2", BumpMinor, "1.1.0"},
		{"1.1.0-rc.2", BumpMajor, "2.0.0"},
		{"1.1.0-rc.2", BumpPatch, "1.1.0"},
		{"1.1.1-rc.1", BumpMinor, "1.2.0"},
		{"2.0.0-rc.1", BumpMajor, "2.0.0"},
	}
	for _, tt := range tests {
		assert.Equal(t, mustVersion(t, tt.in).Inc(tt.t).String(), tt.want, "%s %s", tt.in, tt.t)
	}
}

func TestIncPrerelease(t *testing.T) {
	assert.Equal(t, mustVersion(t, "1.1.0-rc.2").IncPrerelease().String(), "1.1.0-rc.3")
	assert.Equal(t, mustVersion(t, "1.1.0-rc").IncPrerelease().String(), "1.1.0-rc.0")
}
