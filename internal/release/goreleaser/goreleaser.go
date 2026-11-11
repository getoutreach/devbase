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
	"regexp"
	"slices"
	"strings"

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

// envItemRegex matches a NAME=value build environment item.
var envItemRegex = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*=`)

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

	// validTargets are the goos/goarch pairs that GoReleaser builds.
	// It drops the other pairs of the matrix.
	validTargets = []string{
		"aix/ppc64", "android/386", "android/amd64", "android/arm", "android/arm64", "darwin/amd64", "darwin/arm64",
		"dragonfly/amd64", "freebsd/386", "freebsd/amd64", "freebsd/arm", "freebsd/arm64", "illumos/amd64",
		"ios/arm64", "js/wasm", "linux/386", "linux/amd64", "linux/arm", "linux/arm64", "linux/ppc64",
		"linux/ppc64le", "linux/mips", "linux/mipsle", "linux/mips64", "linux/mips64le", "linux/s390x",
		"linux/riscv64", "linux/loong64", "netbsd/386", "netbsd/amd64", "netbsd/arm", "openbsd/386",
		"openbsd/amd64", "openbsd/arm", "openbsd/arm64", "plan9/386", "plan9/amd64", "plan9/arm", "solaris/amd64",
		"windows/arm", "windows/arm64", "windows/386", "windows/amd64",
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
	return cfg.artifacts(), nil
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

// validate checks the config and applies the build defaults.
func (c *config) validate() error {
	if len(c.Archives) > 1 {
		return fmt.Errorf("%w: more than one archive", spec.ErrUnsupported)
	}
	if len(c.Archives) == 1 {
		if f := c.Archives[0].Format; f != "" && f != archiveFormat {
			return fmt.Errorf("%w: archive format %q", spec.ErrUnsupported, f)
		}
	}
	for _, f := range c.archiveFiles() {
		if err := spec.CheckTemplate(f.Src); err != nil {
			return fmt.Errorf("archive file %q: %w", f.Src, err)
		}
		// The builder checks a template src after it renders it.
		if !strings.Contains(f.Src, "{{") {
			if err := spec.ValidateArchiveSrc(f.Src); err != nil {
				return err
			}
		}
	}
	for _, hook := range c.Before.Hooks {
		if err := spec.CheckTemplate(hook); err != nil {
			return fmt.Errorf("before hook %q: %w", hook, err)
		}
	}
	if n := c.Checksum.NameTemplate; n != checksumsFile {
		return fmt.Errorf("%w: checksum name_template %q (only %q)", spec.ErrUnsupported, n, checksumsFile)
	}
	if len(c.Builds) == 0 {
		return fmt.Errorf("%w: no builds (the GoReleaser default build needs the project name)", spec.ErrUnsupported)
	}
	ids := map[string]bool{}
	binaries := map[string]bool{}
	for i := range c.Builds {
		b := &c.Builds[i]
		b.applyDefaults()
		if err := b.validate(); err != nil {
			return fmt.Errorf("build %q: %w", b.ID, err)
		}
		if ids[b.ID] {
			return fmt.Errorf("%w: two builds with id %q", spec.ErrUnsupported, b.ID)
		}
		ids[b.ID] = true
		// GoReleaser puts every binary in one archive, and fails when
		// two have the same name.
		if binaries[b.Binary] {
			return fmt.Errorf("%w: two builds with binary %q", spec.ErrUnsupported, b.Binary)
		}
		binaries[b.Binary] = true
		// Each archive holds every build for one target.
		if i > 0 && !slices.Equal(b.targets(), c.Builds[0].targets()) {
			return fmt.Errorf("%w: build %q has other targets than build %q", spec.ErrUnsupported, b.ID, c.Builds[0].ID)
		}
	}
	return nil
}

// applyDefaults applies the GoReleaser defaults of a build that the
// release command supports.
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

// validate checks one build after applyDefaults.
func (b *build) validate() error {
	switch {
	case b.ID == "":
		return fmt.Errorf("%w: no id (the default is the project name)", spec.ErrUnsupported)
	case b.Binary == "":
		return fmt.Errorf("%w: no binary (the default is the project name)", spec.ErrUnsupported)
	case strings.Contains(b.Binary, "{{"):
		return fmt.Errorf("%w: binary is a template", spec.ErrUnsupported)
	case len(b.LDFlags) == 0:
		return fmt.Errorf("%w: no ldflags (the default needs the commit and the build date)", spec.ErrUnsupported)
	}
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
	for _, f := range b.LDFlags {
		if err := spec.CheckTemplate(f); err != nil {
			return fmt.Errorf("ldflags %q: %w", f, err)
		}
	}
	for _, e := range b.Env {
		if !envItemRegex.MatchString(e) {
			return fmt.Errorf("%w: env item %q is not NAME=value", spec.ErrUnsupported, e)
		}
		if err := spec.CheckTemplate(e); err != nil {
			return fmt.Errorf("env item %q: %w", e, err)
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
			if slices.Contains(validTargets, t.String()) && !b.ignored(&t) {
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
