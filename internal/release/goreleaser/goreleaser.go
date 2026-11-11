// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Loads the subset of .goreleaser.yml that stencil-golang
// renders, and the GoReleaser naming defaults.

// Package goreleaser loads the subset of the GoReleaser configuration
// (.goreleaser.yml) that stencil-golang renders.
package goreleaser

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"slices"

	"github.com/getoutreach/devbase/v2/internal/release/internal/strictyaml"
)

// File is the GoReleaser config file name.
const File = ".goreleaser.yml"

// checksumsFile is the only checksum file name that the loader accepts.
const checksumsFile = "checksums.txt"

// ErrUnsupported is returned for any part of .goreleaser.yml that
// the release command does not support.
var ErrUnsupported = errors.New("unsupported .goreleaser.yml configuration")

// envItemRegex matches a NAME=value build environment item.
var envItemRegex = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*=`)

// defaultArchiveFiles are the GoReleaser default archive files.
//
//nolint:gochecknoglobals // Why: read-only list.
var defaultArchiveFiles = []ArchiveFile{
	{Src: "license*"}, {Src: "LICENSE*"}, {Src: "readme*"}, {Src: "README*"}, {Src: "changelog*"}, {Src: "CHANGELOG*"},
}

// Config is the supported subset of .goreleaser.yml.
type Config struct {
	Before struct {
		Hooks []string `yaml:"hooks"`
	} `yaml:"before"`
	Builds   []Build `yaml:"builds"`
	Archives []struct {
		Files []ArchiveFile `yaml:"files"`
	} `yaml:"archives"`
	Checksum struct {
		NameTemplate string `yaml:"name_template"`
	} `yaml:"checksum"`
	Release struct {
		Disable bool `yaml:"disable"`
	} `yaml:"release"`
}

// Build is one builds entry.
type Build struct {
	Main    string   `yaml:"main"`
	ID      string   `yaml:"id"`
	Binary  string   `yaml:"binary"`
	GOOS    []string `yaml:"goos"`
	GOARCH  []string `yaml:"goarch"`
	Ignore  []Target `yaml:"ignore"`
	LDFlags []string `yaml:"ldflags"`
	Env     []string `yaml:"env"`
}

// Target is a GOOS and GOARCH pair.
type Target struct {
	OS   string `yaml:"goos"`
	Arch string `yaml:"goarch"`
}

// String returns the target as "os/arch".
func (t Target) String() string {
	return t.OS + "/" + t.Arch
}

// ArchiveFile is one archives[].files entry.
type ArchiveFile struct {
	Src  string `yaml:"src"`
	Dst  string `yaml:"dst"`
	Info struct {
		Mode os.FileMode `yaml:"mode"`
	} `yaml:"info"`
}

// Parse parses and validates a .goreleaser.yml file.
func Parse(b []byte) (*Config, error) {
	var cfg Config
	if err := strictyaml.Decode(b, &cfg); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrUnsupported, err)
	}
	if len(cfg.Archives) > 1 {
		return nil, fmt.Errorf("%w: more than one archive", ErrUnsupported)
	}
	if n := cfg.Checksum.NameTemplate; n != "" && n != checksumsFile {
		return nil, fmt.Errorf("%w: checksum name_template %q", ErrUnsupported, n)
	}
	for i := range cfg.Builds {
		b := &cfg.Builds[i]
		if b.Binary == "" {
			b.Binary = b.ID
		}
		for _, e := range b.Env {
			if !envItemRegex.MatchString(e) {
				return nil, fmt.Errorf("%w: build %q env item %q is not NAME=value", ErrUnsupported, b.ID, e)
			}
		}
	}
	return &cfg, nil
}

// Targets returns the targets of the first build: the goos/goarch
// matrix minus the ignore entries. All builds must have the same
// targets, because each archive holds every build for one target.
func (c *Config) Targets() []Target {
	if len(c.Builds) == 0 {
		return nil
	}
	b := &c.Builds[0]
	var out []Target
	for _, goos := range b.GOOS {
		for _, arch := range b.GOARCH {
			if t := (Target{OS: goos, Arch: arch}); !slices.Contains(b.Ignore, t) {
				out = append(out, t)
			}
		}
	}
	return out
}

// ArchiveFiles returns the extra files of the archive: the configured
// files, or the GoReleaser defaults.
func (c *Config) ArchiveFiles() []ArchiveFile {
	if len(c.Archives) == 1 && len(c.Archives[0].Files) > 0 {
		return c.Archives[0].Files
	}
	return defaultArchiveFiles
}
