// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Loads the subset of .goreleaser.yml that stencil-golang
// renders, with the GoReleaser 1.20 defaults and target rules.

// Package goreleaser loads the subset of the GoReleaser configuration
// (.goreleaser.yml) that stencil-golang renders into spec.Artifacts.
package goreleaser

import (
	"bytes"
	"fmt"
	"os"
	"slices"

	"github.com/getoutreach/devbase/v2/internal/release/internal/strictyaml"
	"github.com/getoutreach/devbase/v2/internal/release/spec"
	"go.yaml.in/yaml/v3"
)

// File is the GoReleaser config file name.
const File = ".goreleaser.yml"

// checksumsFile is the only checksum file name that the loader accepts.
const checksumsFile = "checksums.txt"

// archiveFormat is the only archive format that the loader accepts. It
// is the GoReleaser default.
const archiveFormat = "tar.gz"

// These are the GoReleaser 1.20 defaults and valid values, from
// internal/builders/golang/build.go and internal/builders/buildtarget.
//
//nolint:gochecknoglobals // Why: read-only lists.
var (
	// defaultArchiveFiles are the GoReleaser default archive files.
	defaultArchiveFiles = []archiveFile{
		{Src: "license*"}, {Src: "LICENSE*"}, {Src: "readme*"}, {Src: "README*"}, {Src: "changelog*"}, {Src: "CHANGELOG*"},
	}

	// defaultGOOS is the default goos list.
	defaultGOOS = []string{"linux", "darwin", "windows"}

	// defaultGOARCH is the default goarch list.
	defaultGOARCH = []string{"amd64", "arm64", "386"}

	// validGOOS are the goos values that GoReleaser accepts.
	validGOOS = []string{
		"aix", "android", "darwin", "dragonfly", "freebsd", "illumos", "ios", "js", "linux", "netbsd", "openbsd",
		"plan9", "solaris", "windows",
	}

	// validGOARCH are the goarch values that GoReleaser accepts.
	validGOARCH = []string{
		"386", "amd64", "arm", "arm64", "mips", "mips64", "mips64le", "mipsle", "ppc64", "ppc64le", "s390x", "wasm",
		"riscv64", "loong64",
	}
)

// config is the supported subset of .goreleaser.yml.
type config struct {
	Before struct {
		Hooks []string `yaml:"hooks"`
	} `yaml:"before"`
	Builds   []build `yaml:"builds"`
	Archives []struct {
		Format string        `yaml:"format"`
		Files  []archiveFile `yaml:"files"`
	} `yaml:"archives"`
	Checksum struct {
		NameTemplate string `yaml:"name_template"`
	} `yaml:"checksum"`
	Release struct {
		Disable bool `yaml:"disable"`
	} `yaml:"release"`
}

// build is one builds entry.
type build struct {
	Main   string   `yaml:"main"`
	ID     string   `yaml:"id"`
	Binary string   `yaml:"binary"`
	GOOS   []string `yaml:"goos"`
	GOARCH []string `yaml:"goarch"`
	Ignore []ignore `yaml:"ignore"`

	// LDFlags are templates, joined with spaces. GoReleaser also accepts
	// one string.
	LDFlags stringList `yaml:"ldflags"`

	// Env items are NAME=value templates.
	Env []string `yaml:"env"`
}

// ignore is one builds[].ignore entry. An empty field matches any value.
type ignore struct {
	OS   string `yaml:"goos"`
	Arch string `yaml:"goarch"`
}

// stringList is a list of strings that can also be written as one string.
type stringList []string

// UnmarshalYAML decodes a string or a list of strings.
func (l *stringList) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind == yaml.ScalarNode {
		*l = stringList{n.Value}
		return nil
	}
	var s []string
	if err := n.Decode(&s); err != nil {
		return err
	}
	*l = s
	return nil
}

// archiveFile is one archives[].files entry. GoReleaser also accepts a
// string, which is the src.
type archiveFile struct {
	Src  string `yaml:"src"`
	Dst  string `yaml:"dst"`
	Info struct {
		Mode os.FileMode `yaml:"mode"`
	} `yaml:"info"`
}

// UnmarshalYAML decodes a string or an object, and rejects unknown keys.
func (f *archiveFile) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind == yaml.ScalarNode {
		*f = archiveFile{Src: n.Value}
		return nil
	}
	type plain archiveFile
	return strictyaml.DecodeNode(n, (*plain)(f))
}

// Parse parses and validates a .goreleaser.yml file, applies the
// GoReleaser defaults that the release command supports, and returns the
// artifacts. Options with a default that needs data the release command
// does not have (such as the project name or the build date) are
// required.
func Parse(b []byte) (*spec.Artifacts, error) {
	var cfg config
	if len(bytes.TrimSpace(b)) > 0 {
		if err := strictyaml.Decode(b, &cfg); err != nil {
			return nil, fmt.Errorf("%s: %w: %w", File, spec.ErrUnsupported, err)
		}
	}
	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("%s: %w", File, err)
	}
	a := cfg.artifacts()
	if err := a.Validate(); err != nil {
		return nil, fmt.Errorf("%s: %w", File, err)
	}
	return a, nil
}

// artifacts maps a validated config into the release model.
func (c *config) artifacts() *spec.Artifacts {
	a := &spec.Artifacts{
		Hooks:     c.Before.Hooks,
		Targets:   c.Builds[0].targets(),
		Checksums: c.Checksum.NameTemplate,
	}
	for i := range c.Builds {
		b := &c.Builds[i]
		a.Builds = append(a.Builds, spec.Build{ID: b.ID, Binary: b.Binary, Main: b.Main, LDFlags: b.LDFlags, Env: b.Env})
	}
	for _, f := range c.archiveFiles() {
		a.Files = append(a.Files, spec.ArchiveFile{Src: f.Src, Dst: f.Dst, Mode: f.Info.Mode})
	}
	return a
}

// archiveFiles returns the extra files of the archive: the configured
// files, or the GoReleaser defaults.
func (c *config) archiveFiles() []archiveFile {
	if len(c.Archives) == 1 && len(c.Archives[0].Files) > 0 {
		return c.Archives[0].Files
	}
	return defaultArchiveFiles
}

// validate checks the parts of the format that the model cannot see: one
// tar.gz archive, the checksum name, the GoReleaser goos and goarch
// values, and the same targets for every build, because the model has
// one target list. It also applies the build defaults. The model checks
// the rest.
func (c *config) validate() error {
	if len(c.Archives) > 1 {
		return fmt.Errorf("%w: more than one archive", spec.ErrUnsupported)
	}
	if len(c.Archives) == 1 {
		if f := c.Archives[0].Format; f != "" && f != archiveFormat {
			return fmt.Errorf("%w: archive format %q", spec.ErrUnsupported, f)
		}
	}
	if n := c.Checksum.NameTemplate; n != checksumsFile {
		return fmt.Errorf("%w: checksum name_template %q (only %q)", spec.ErrUnsupported, n, checksumsFile)
	}
	if len(c.Builds) == 0 {
		return fmt.Errorf("%w: no builds (the GoReleaser default build needs the project name)", spec.ErrUnsupported)
	}
	for i := range c.Builds {
		b := &c.Builds[i]
		b.applyDefaults()
		if err := b.validate(); err != nil {
			return fmt.Errorf("build %q: %w", b.ID, err)
		}
		if i > 0 && !slices.Equal(b.targets(), c.Builds[0].targets()) {
			return fmt.Errorf("%w: build %q has other targets than build %q", spec.ErrUnsupported, b.ID, c.Builds[0].ID)
		}
	}
	return nil
}

// applyDefaults applies the GoReleaser defaults of a build that the
// release command supports. The defaults of id, binary and ldflags need
// the project name, the commit or the build date, so they stay empty and
// the model rejects them.
func (b *build) applyDefaults() {
	if b.Main == "" {
		b.Main = "."
	}
	if len(b.GOOS) == 0 {
		b.GOOS = defaultGOOS
	}
	if len(b.GOARCH) == 0 {
		b.GOARCH = defaultGOARCH
	}
}

// validate checks the goos and goarch values of a build.
func (b *build) validate() error {
	for _, goos := range b.GOOS {
		if !slices.Contains(validGOOS, goos) {
			return fmt.Errorf("%w: invalid goos: %s", spec.ErrUnsupported, goos)
		}
	}
	for _, arch := range b.GOARCH {
		if !slices.Contains(validGOARCH, arch) {
			return fmt.Errorf("%w: invalid goarch: %s", spec.ErrUnsupported, arch)
		}
	}
	return nil
}

// targets returns the targets of a build: the valid goos/goarch pairs
// of the matrix that no ignore entry matches.
func (b *build) targets() []spec.Target {
	var out []spec.Target
	for _, goos := range b.GOOS {
		for _, arch := range b.GOARCH {
			t := spec.NewTarget(goos, arch)
			if t.Supported() && !b.ignored(&t) {
				out = append(out, t)
			}
		}
	}
	return out
}

// ignored reports whether an ignore entry matches t.
func (b *build) ignored(t *spec.Target) bool {
	return slices.ContainsFunc(b.Ignore, func(ig ignore) bool {
		return (ig.OS == "" || ig.OS == t.OS) && (ig.Arch == "" || ig.Arch == t.Arch)
	})
}
