// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: The artifacts of a release: Go builds, their targets, and
// the files of the tar.gz archive of each target.

package spec

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
)

// Artifacts are the binaries and archives of a release. Every build is
// built for every target, and each target has one tar.gz archive with
// the extra files and the binaries of all builds.
type Artifacts struct {
	// Hooks are commands to run once before the builds. Each is a
	// template, split into arguments as a shell does, and run without a
	// shell.
	Hooks []string

	// Builds are the Go programs to build.
	Builds []Build

	// Targets are the targets of every build.
	Targets []Target

	// Files are the extra files of each archive.
	Files []ArchiveFile

	// Checksums is the name of the checksum file.
	Checksums string
}

// Build is one Go program.
type Build struct {
	// ID names the build. Binaries are built in a folder with this name.
	ID string

	// Binary is the binary name, without the target's file extension.
	Binary string

	// Main is the main package, as `go build` takes it.
	Main string

	// LDFlags are templates, joined with spaces.
	LDFlags []string

	// Env items are NAME=value templates. Each item sees the items
	// before it in .Env.
	Env []string
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

// ArchiveFile is an extra file of the archives.
type ArchiveFile struct {
	// Src is a template that renders a glob relative to the repository.
	// A matched folder adds every file in it.
	Src string

	// Dst is the folder of the matched files in the archive. Empty
	// means the path in the repository.
	Dst string

	// Mode is the file mode in the archive, or zero for the file's mode.
	Mode fs.FileMode
}

// Glob renders the src template with .Env from env and returns the glob,
// relative to the repository. It rejects a glob that ValidateArchiveSrc
// rejects.
func (f *ArchiveFile) Glob(t *Template, env []string) (string, error) {
	glob, err := t.Apply(f.Src, env)
	if err != nil {
		return "", fmt.Errorf("archive file %q: %w", f.Src, err)
	}
	if err := ValidateArchiveSrc(glob); err != nil {
		return "", err
	}
	return glob, nil
}

// ValidateArchiveSrc rejects an archive file glob that the release
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
func ValidateArchiveSrc(src string) error {
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
