// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Tests for the .goreleaser.yml loader.

package goreleaser

import (
	"strings"
	"testing"

	"gotest.tools/v3/assert"
)

// fixture is a stencil-golang style .goreleaser.yml.
const fixture = `before:
  hooks: [make dep]
builds:
  - main: ./cmd/orc
    id: &name orc
    binary: *name
    goos: [linux, darwin]
    goarch: [amd64, arm64]
    ignore:
      - goos: darwin
        goarch: amd64
    ldflags:
      - '-w -s -X "main.version=v{{ .Version }}"'
    env:
      - CGO_ENABLED=0
archives: []
checksum:
  name_template: "checksums.txt"
release:
  disable: true
`

func TestParse(t *testing.T) {
	cfg, err := Parse([]byte(fixture))
	assert.NilError(t, err)
	assert.DeepEqual(t, cfg.Targets(), []Target{{OS: "linux", Arch: "amd64"}, {OS: "linux", Arch: "arm64"}, {OS: "darwin", Arch: "arm64"}})
	assert.DeepEqual(t, cfg.ArchiveFiles(), defaultArchiveFiles)
}

func TestParseRejectsBadEnv(t *testing.T) {
	cfg := strings.Replace(fixture, "      - CGO_ENABLED=0\n",
		"      - CGO_ENABLED=0\n      - '-X \"main.secret={{ .Env.SECRET }}\"'\n", 1)
	_, err := Parse([]byte(cfg))
	assert.ErrorIs(t, err, ErrUnsupported)
}

func TestParseRejectsUnsupported(t *testing.T) {
	for name, cfg := range map[string]string{
		"unknown key":       fixture + "nfpms: []\n",
		"checksum template": strings.Replace(fixture, `"checksums.txt"`, `"{{ .ProjectName }}_checksums.txt"`, 1),
	} {
		_, err := Parse([]byte(cfg))
		assert.ErrorIs(t, err, ErrUnsupported, name)
	}
}
