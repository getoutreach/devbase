// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Tests for the build targets of the release model.

package spec_test

import (
	"testing"

	"github.com/getoutreach/devbase/v2/internal/release/spec"
	"gotest.tools/v3/assert"
)

func TestNewTarget(t *testing.T) {
	for _, tc := range []struct {
		arch        string
		want        spec.Target
		env         []string
		archiveArch string
	}{
		{
			"arm",
			spec.Target{OS: "linux", Arch: "arm", Goarm: "6"},
			[]string{"GOARM=6", "GOMIPS=", "GOMIPS64=", "GOAMD64="},
			"armv6",
		},
		{
			"mips",
			spec.Target{OS: "linux", Arch: "mips", Gomips: "hardfloat"},
			[]string{"GOARM=", "GOMIPS=hardfloat", "GOMIPS64=hardfloat", "GOAMD64="},
			"mips_hardfloat",
		},
		{
			"mips64le",
			spec.Target{OS: "linux", Arch: "mips64le", Gomips: "hardfloat"},
			[]string{"GOARM=", "GOMIPS=hardfloat", "GOMIPS64=hardfloat", "GOAMD64="},
			"mips64le_hardfloat",
		},
		{
			"amd64",
			spec.Target{OS: "linux", Arch: "amd64", Goamd64: "v1"},
			[]string{"GOARM=", "GOMIPS=", "GOMIPS64=", "GOAMD64=v1"},
			"amd64",
		},
		{"arm64", spec.Target{OS: "linux", Arch: "arm64"}, []string{"GOARM=", "GOMIPS=", "GOMIPS64=", "GOAMD64="}, "arm64"},
		{"386", spec.Target{OS: "linux", Arch: "386"}, []string{"GOARM=", "GOMIPS=", "GOMIPS64=", "GOAMD64="}, "386"},
	} {
		t.Run(tc.arch, func(t *testing.T) {
			got := spec.NewTarget("linux", tc.arch)
			assert.DeepEqual(t, got, tc.want)
			assert.DeepEqual(t, got.Env(), append([]string{"GOOS=linux", "GOARCH=" + tc.arch}, tc.env...))
			assert.Equal(t, got.ArchiveArch(), tc.archiveArch)
			assert.Equal(t, got.String(), "linux/"+tc.arch)
		})
	}

	t.Run("ArchiveArch with goamd64 v3", func(t *testing.T) {
		// The default name template adds goamd64 unless it is v1.
		assert.Equal(t, (&spec.Target{OS: "linux", Arch: "amd64", Goamd64: "v3"}).ArchiveArch(), "amd64v3")
	})
}
