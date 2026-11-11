// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Loads the subset of .goreleaser.yml that stencil-golang
// renders, with the GoReleaser 1.20 defaults and target rules.

// Package goreleaser loads the subset of the GoReleaser configuration
// (.goreleaser.yml) that stencil-golang renders.
package goreleaser

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"text/template"
	"text/template/parse"

	"github.com/getoutreach/devbase/v2/internal/release/internal/strictyaml"
	"go.yaml.in/yaml/v3"
)

// File is the GoReleaser config file name.
const File = ".goreleaser.yml"

// checksumsFile is the only checksum file name that the loader accepts.
const checksumsFile = "checksums.txt"

// archiveFormat is the only archive format that the loader accepts. It
// is the GoReleaser default.
const archiveFormat = "tar.gz"

// ErrUnsupported is returned for any part of .goreleaser.yml that
// the release command does not support.
var ErrUnsupported = errors.New("unsupported .goreleaser.yml configuration")

// envItemRegex matches a NAME=value build environment item.
var envItemRegex = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*=`)

// These are the GoReleaser 1.20 defaults and valid values, from
// internal/builders/golang/build.go and internal/builders/buildtarget.
//
//nolint:gochecknoglobals // Why: read-only lists.
var (
	// defaultArchiveFiles are the GoReleaser default archive files.
	defaultArchiveFiles = []ArchiveFile{
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

// Config is the supported subset of .goreleaser.yml.
type Config struct {
	Before struct {
		Hooks []string `yaml:"hooks"`
	} `yaml:"before"`
	Builds   []Build `yaml:"builds"`
	Archives []struct {
		Format string        `yaml:"format"`
		Files  []ArchiveFile `yaml:"files"`
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
	Main   string   `yaml:"main"`
	ID     string   `yaml:"id"`
	Binary string   `yaml:"binary"`
	GOOS   []string `yaml:"goos"`
	GOARCH []string `yaml:"goarch"`
	Ignore []Ignore `yaml:"ignore"`

	// LDFlags are templates, joined with spaces. GoReleaser also accepts
	// one string.
	LDFlags StringList `yaml:"ldflags"`

	// Env items are NAME=value templates.
	Env []string `yaml:"env"`
}

// Ignore is one builds[].ignore entry. An empty field matches any value.
type Ignore struct {
	OS   string `yaml:"goos"`
	Arch string `yaml:"goarch"`
}

// StringList is a list of strings that can also be written as one string.
type StringList []string

// UnmarshalYAML decodes a string or a list of strings.
func (l *StringList) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind == yaml.ScalarNode {
		*l = StringList{n.Value}
		return nil
	}
	var s []string
	if err := n.Decode(&s); err != nil {
		return err
	}
	*l = s
	return nil
}

// Target is one build target: a GOOS and GOARCH pair, with the variant
// that GoReleaser builds by default. Use NewTarget to make one.
type Target struct {
	OS   string
	Arch string

	// Goarm, Gomips and Goamd64 are the variant of an arm, mips or amd64
	// target. They are empty when they do not apply.
	Goarm   string
	Gomips  string
	Goamd64 string
}

// NewTarget returns the target for goos and goarch with the GoReleaser
// default variant: goarm 6, gomips hardfloat and goamd64 v1, as in
// allBuildTargets of internal/builders/buildtarget.
func NewTarget(goos, goarch string) Target {
	t := Target{OS: goos, Arch: goarch}
	switch {
	case goarch == "arm":
		t.Goarm = "6"
	case strings.HasPrefix(goarch, "amd64"):
		t.Goamd64 = "v1"
	case strings.HasPrefix(goarch, "mips"):
		t.Gomips = "hardfloat"
	}
	return t
}

// String returns the target as "os/arch".
func (t *Target) String() string {
	return t.OS + "/" + t.Arch
}

// Env returns the variables that GoReleaser sets for the target. It sets
// every variant variable, empty when it does not apply, so that the
// process environment does not change the build.
func (t *Target) Env() []string {
	return []string{
		"GOOS=" + t.OS,
		"GOARCH=" + t.Arch,
		"GOARM=" + t.Goarm,
		"GOMIPS=" + t.Gomips,
		"GOMIPS64=" + t.Gomips,
		"GOAMD64=" + t.Goamd64,
	}
}

// ArchiveArch returns the architecture part of the default archive name
// template: {{ .Arch }}{{ with .Arm }}v{{ . }}{{ end }}{{ with .Mips
// }}_{{ . }}{{ end }}{{ if not (eq .Amd64 "v1") }}{{ .Amd64 }}{{ end }}.
func (t *Target) ArchiveArch() string {
	s := t.Arch
	if t.Goarm != "" {
		s += "v" + t.Goarm
	}
	if t.Gomips != "" {
		s += "_" + t.Gomips
	}
	if t.Goamd64 != "v1" {
		s += t.Goamd64
	}
	return s
}

// BinaryExt returns the file extension of a binary for the target.
func (t *Target) BinaryExt() string {
	switch {
	case t.OS == "js" && t.Arch == "wasm":
		return ".wasm"
	case t.OS == "windows":
		return ".exe"
	}
	return ""
}

// ArchiveFile is one archives[].files entry. GoReleaser also accepts a
// string, which is the src.
type ArchiveFile struct {
	Src  string `yaml:"src"`
	Dst  string `yaml:"dst"`
	Info struct {
		Mode os.FileMode `yaml:"mode"`
	} `yaml:"info"`
}

// UnmarshalYAML decodes a string or an object, and rejects unknown keys.
func (f *ArchiveFile) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind == yaml.ScalarNode {
		*f = ArchiveFile{Src: n.Value}
		return nil
	}
	type plain ArchiveFile
	return strictyaml.DecodeNode(n, (*plain)(f))
}

// Glob renders the src template with .Env from env and returns the glob,
// relative to the repository. It rejects a glob that the release command
// does not resolve as GoReleaser does.
func (f *ArchiveFile) Glob(t *Template, env []string) (string, error) {
	glob, err := t.Apply(f.Src, env)
	if err != nil {
		return "", fmt.Errorf("archive file %q: %w", f.Src, err)
	}
	if err := validateArchiveSrc(glob); err != nil {
		return "", err
	}
	return glob, nil
}

// Parse parses and validates a .goreleaser.yml file, and applies the
// GoReleaser defaults that the release command supports. Options with
// a default that needs data the release command does not have (such as
// the project name or the build date) are required.
func Parse(b []byte) (*Config, error) {
	var cfg Config
	if len(bytes.TrimSpace(b)) > 0 {
		if err := strictyaml.Decode(b, &cfg); err != nil {
			return nil, fmt.Errorf("%w: %w", ErrUnsupported, err)
		}
	}
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// Targets returns the targets of the builds. Parse checks that all
// builds have the same targets.
func (c *Config) Targets() []Target {
	if len(c.Builds) == 0 {
		return nil
	}
	return c.Builds[0].targets()
}

// ArchiveFiles returns the extra files of the archive: the configured
// files, or the GoReleaser defaults.
func (c *Config) ArchiveFiles() []ArchiveFile {
	if len(c.Archives) == 1 && len(c.Archives[0].Files) > 0 {
		return c.Archives[0].Files
	}
	return defaultArchiveFiles
}

// validate checks the config and applies the build defaults.
func (c *Config) validate() error {
	if len(c.Archives) > 1 {
		return fmt.Errorf("%w: more than one archive", ErrUnsupported)
	}
	if len(c.Archives) == 1 {
		if f := c.Archives[0].Format; f != "" && f != archiveFormat {
			return fmt.Errorf("%w: archive format %q", ErrUnsupported, f)
		}
	}
	for _, f := range c.ArchiveFiles() {
		if err := checkTemplate(f.Src); err != nil {
			return fmt.Errorf("archive file %q: %w", f.Src, err)
		}
		// The builder checks a template src after it renders it.
		if !strings.Contains(f.Src, "{{") {
			if err := validateArchiveSrc(f.Src); err != nil {
				return err
			}
		}
	}
	for _, hook := range c.Before.Hooks {
		if err := checkTemplate(hook); err != nil {
			return fmt.Errorf("before hook %q: %w", hook, err)
		}
	}
	if n := c.Checksum.NameTemplate; n != checksumsFile {
		return fmt.Errorf("%w: checksum name_template %q (only %q)", ErrUnsupported, n, checksumsFile)
	}
	if len(c.Builds) == 0 {
		return fmt.Errorf("%w: no builds (the GoReleaser default build needs the project name)", ErrUnsupported)
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
			return fmt.Errorf("%w: two builds with id %q", ErrUnsupported, b.ID)
		}
		ids[b.ID] = true
		// GoReleaser puts every binary in one archive, and fails when
		// two have the same name.
		if binaries[b.Binary] {
			return fmt.Errorf("%w: two builds with binary %q", ErrUnsupported, b.Binary)
		}
		binaries[b.Binary] = true
		// Each archive holds every build for one target.
		if i > 0 && !slices.Equal(b.targets(), c.Builds[0].targets()) {
			return fmt.Errorf("%w: build %q has other targets than build %q", ErrUnsupported, b.ID, c.Builds[0].ID)
		}
	}
	return nil
}

// applyDefaults applies the GoReleaser defaults of a build that the
// release command supports.
func (b *Build) applyDefaults() {
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
func (b *Build) validate() error {
	switch {
	case b.ID == "":
		return fmt.Errorf("%w: no id (the default is the project name)", ErrUnsupported)
	case b.Binary == "":
		return fmt.Errorf("%w: no binary (the default is the project name)", ErrUnsupported)
	case strings.Contains(b.Binary, "{{"):
		return fmt.Errorf("%w: binary is a template", ErrUnsupported)
	case len(b.LDFlags) == 0:
		return fmt.Errorf("%w: no ldflags (the default needs the commit and the build date)", ErrUnsupported)
	}
	for _, goos := range b.GOOS {
		if !slices.Contains(validGOOS, goos) {
			return fmt.Errorf("%w: invalid goos: %s", ErrUnsupported, goos)
		}
	}
	for _, arch := range b.GOARCH {
		if !slices.Contains(validGOARCH, arch) {
			return fmt.Errorf("%w: invalid goarch: %s", ErrUnsupported, arch)
		}
	}
	for _, f := range b.LDFlags {
		if err := checkTemplate(f); err != nil {
			return fmt.Errorf("ldflags %q: %w", f, err)
		}
	}
	for _, e := range b.Env {
		if !envItemRegex.MatchString(e) {
			return fmt.Errorf("%w: env item %q is not NAME=value", ErrUnsupported, e)
		}
		if err := checkTemplate(e); err != nil {
			return fmt.Errorf("env item %q: %w", e, err)
		}
	}
	return nil
}

// validateArchiveSrc rejects an archive file glob that the release
// command does not resolve as GoReleaser does with fileglob.Glob, which
// trims a leading "./" and a trailing "/":
//   - "**", "{a,b}" and "[!a]" match more in fileglob than in
//     filepath.Glob, "[^a]" matches less, and the builder does not
//     handle "\" escapes.
//   - "../" and "/" globs read outside the repository.
//   - fileglob fails on a "." or ".." element ("a/../b"), and matches
//     nothing for an empty element ("a//b"); filepath.Glob cleans them.
//   - An empty glob matches nothing in fileglob, and the repository in
//     filepath.Glob. "." matches the repository, with the files that
//     GoReleaser writes to dist while it archives.
func validateArchiveSrc(src string) error {
	p := strings.TrimSuffix(strings.TrimPrefix(src, "./"), "/")
	var reason string
	switch {
	case strings.Contains(src, "**"):
		reason = "** globs"
	case strings.ContainsAny(src, `{}\`), strings.Contains(src, "[!"), strings.Contains(src, "[^"):
		reason = `{, }, \, [! or [^ in a glob`
	case p == "":
		reason = "empty"
	case p == ".":
		reason = "the repository"
	case !filepath.IsLocal(filepath.FromSlash(p)):
		reason = "outside the repository"
	case !fs.ValidPath(p):
		reason = `a ".", ".." or empty element`
	default:
		return nil
	}
	return fmt.Errorf("%w: archive file %q: %s", ErrUnsupported, src, reason)
}

// targets returns the targets of a build: the valid goos/goarch pairs
// of the matrix that no ignore entry matches.
func (b *Build) targets() []Target {
	var out []Target
	for _, goos := range b.GOOS {
		for _, arch := range b.GOARCH {
			t := NewTarget(goos, arch)
			if slices.Contains(validTargets, t.String()) && !b.ignored(&t) {
				out = append(out, t)
			}
		}
	}
	return out
}

// ignored reports whether an ignore entry matches t.
func (b *Build) ignored(t *Target) bool {
	return slices.ContainsFunc(b.Ignore, func(ig Ignore) bool {
		return (ig.OS == "" || ig.OS == t.OS) && (ig.Arch == "" || ig.Arch == t.Arch)
	})
}

// Template is the data of a GoReleaser template: the fields that the
// release command supports. .Env comes from the caller.
type Template struct {
	Version string
	Tag     string
}

// Apply renders a GoReleaser template with .Env from env. It rejects a
// template that parseTemplate rejects with ErrUnsupported. A missing key
// is an error, as in GoReleaser.
func (t *Template) Apply(s string, env []string) (string, error) {
	tmpl, err := parseTemplate(s)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	data := map[string]any{"Version": t.Version, "Tag": t.Tag, "Env": EnvMap(env)}
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("execute template: %w", err)
	}
	return buf.String(), nil
}

// EnvMap returns the NAME=value items of env as a map, which is .Env in
// a template. Later items win, and items with no name are left out, as
// in GoReleaser.
func EnvMap(env []string) map[string]string {
	m := map[string]string{}
	for _, e := range env {
		if k, v, ok := strings.Cut(e, "="); ok && k != "" {
			m[k] = v
		}
	}
	return m
}

// checkTemplate rejects a template that the release command cannot
// render as GoReleaser does.
func checkTemplate(s string) error {
	_, err := parseTemplate(s)
	return err
}

// parseTemplate parses a template as GoReleaser does, and rejects it
// unless it uses only the fields .Version, .Tag, .Env and .Env.<NAME>,
// with text, variables, dot, literals, pipelines, and if, range and with.
// GoReleaser has more fields and functions, which the release command
// does not have the data for.
func parseTemplate(s string) (*template.Template, error) {
	tmpl, err := template.New("tmpl").Option("missingkey=error").Parse(s)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrUnsupported, err)
	}
	if len(tmpl.Templates()) != 1 {
		return nil, fmt.Errorf("%w: define in a template", ErrUnsupported)
	}
	if err := checkTemplateNode(tmpl.Root); err != nil {
		return nil, err
	}
	return tmpl, nil
}

// checkTemplateNode returns an error that names the first node below n
// that parseTemplate does not allow.
func checkTemplateNode(n parse.Node) error {
	switch n := n.(type) {
	case *parse.ListNode:
		if n == nil {
			return nil
		}
		for _, c := range n.Nodes {
			if err := checkTemplateNode(c); err != nil {
				return err
			}
		}
	case *parse.ActionNode:
		return checkTemplateNode(n.Pipe)
	case *parse.IfNode:
		return checkBranchNode(&n.BranchNode)
	case *parse.RangeNode:
		return checkBranchNode(&n.BranchNode)
	case *parse.WithNode:
		return checkBranchNode(&n.BranchNode)
	case *parse.PipeNode:
		for _, c := range n.Cmds {
			if err := checkTemplateNode(c); err != nil {
				return err
			}
		}
	case *parse.CommandNode:
		for _, a := range n.Args {
			if err := checkTemplateNode(a); err != nil {
				return err
			}
		}
	case *parse.FieldNode:
		return checkTemplateField(n.Ident)
	case *parse.VariableNode:
		// Only $ has the template fields.
		if len(n.Ident) > 1 && n.Ident[0] != "$" {
			return fmt.Errorf("%w: field %s", ErrUnsupported, n)
		}
		if len(n.Ident) > 1 {
			return checkTemplateField(n.Ident[1:])
		}
	case *parse.IdentifierNode:
		return fmt.Errorf("%w: function %q", ErrUnsupported, n.Ident)
	case *parse.TextNode, *parse.CommentNode, *parse.DotNode, *parse.StringNode, *parse.NumberNode,
		*parse.BoolNode, *parse.NilNode, *parse.BreakNode, *parse.ContinueNode:
	default:
		return fmt.Errorf("%w: %q", ErrUnsupported, n)
	}
	return nil
}

// checkBranchNode checks the pipeline and both lists of an if, range or
// with node.
func checkBranchNode(n *parse.BranchNode) error {
	for _, c := range []parse.Node{n.Pipe, n.List, n.ElseList} {
		if err := checkTemplateNode(c); err != nil {
			return err
		}
	}
	return nil
}

// checkTemplateField allows the fields .Version, .Tag, .Env and
// .Env.<NAME>.
func checkTemplateField(ident []string) error {
	switch {
	case len(ident) == 1 && slices.Contains([]string{"Version", "Tag", "Env"}, ident[0]),
		len(ident) == 2 && ident[0] == "Env":
		return nil
	}
	return fmt.Errorf("%w: field .%s", ErrUnsupported, strings.Join(ident, "."))
}
