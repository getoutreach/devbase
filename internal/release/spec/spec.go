// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: The release model: branches, release rules, plugins and
// artifacts, independent of the config file format and of the forge.

// Package spec is the typed release model. The loaders of the release
// section of scripts/devbase.yaml, of .releaserc.yaml and of
// .goreleaser.yml map their file formats into it, and the version engine,
// the notes renderer and the builder use only it.
package spec

import (
	"errors"
	"slices"
)

// ErrUnsupported is returned for a value that the release command
// cannot reproduce. Loaders wrap it in their own error.
var ErrUnsupported = errors.New("unsupported release configuration")

// Config is the release configuration of a repository.
type Config struct {
	// Branches are the release and prerelease branches, in order.
	Branches []Branch

	// Rules are the custom release rules. A commit that matches no
	// custom rule uses the default rules.
	Rules []Rule

	// MajorCheck is true when major versions are blocked.
	MajorCheck bool

	// Build is true when the release builds Artifacts. The build command
	// does it, before the publish step.
	Build bool

	// Plugins are the release capabilities, in config order. The order
	// sets the order of hooks in each step.
	Plugins []Plugin

	// Artifacts are the binaries and archives to build, or nil.
	Artifacts *Artifacts

	// Units are the extra release units, in release order. The root
	// release is not one of them.
	Units []Unit

	// RootPaths are the path rules of the root release when there are
	// units.
	RootPaths RootPaths
}

// Unit returns the unit with the given name, or nil.
func (c *Config) Unit(name string) *Unit {
	for i := range c.Units {
		if c.Units[i].Name == name {
			return &c.Units[i]
		}
	}
	return nil
}

// Branch returns the configured branch with the given name, or nil.
func (c *Config) Branch(name string) *Branch {
	for i := range c.Branches {
		if c.Branches[i].Name == name {
			return &c.Branches[i]
		}
	}
	return nil
}

// Bump is the size of a version bump: the release type.
type Bump string

// These are the bumps, in increasing order of size.
const (
	// BumpNone means that no release is needed.
	BumpNone Bump = ""

	// BumpPatch is a patch release.
	BumpPatch Bump = "patch"

	// BumpMinor is a minor release.
	BumpMinor Bump = "minor"

	// BumpMajor is a major release.
	BumpMajor Bump = "major"

	// BumpNoRelease is the Release of a rule that stops a release. It
	// has no rank, so it replaces any bump and any bump replaces it, as
	// "false" does in commit-analyzer.
	BumpNoRelease Bump = "false"
)

// bumpOrder lists the bumps from smallest to largest.
//
//nolint:gochecknoglobals // Why: read-only lookup table.
var bumpOrder = []Bump{BumpNone, BumpPatch, BumpMinor, BumpMajor}

// Rank orders bumps so that the larger one can be chosen. An unknown
// bump has rank -1.
func (b Bump) Rank() int {
	return slices.Index(bumpOrder, b)
}

// MaxBump returns the larger of two bumps.
func MaxBump(a, b Bump) Bump {
	if b.Rank() > a.Rank() {
		return b
	}
	return a
}

// Branch is a release or prerelease branch.
type Branch struct {
	// Name is the git branch name.
	Name string `json:"name"`

	// Prerelease is the prerelease identifier (for example "rc"), or
	// empty for the release branch.
	Prerelease string `json:"prerelease,omitempty"`

	// Channel is the channel name. The release branch uses the default
	// channel, which is the empty string.
	Channel string `json:"channel,omitempty"`
}

// IsPrerelease reports whether b is a prerelease branch.
func (b *Branch) IsPrerelease() bool {
	return b.Prerelease != ""
}

// Rule is a release rule. A rule matches a commit when every set field
// matches; Type, Scope and Subject are globs (see MatchGlob).
type Rule struct {
	Type     string
	Scope    string
	Subject  string
	Breaking bool
	Revert   bool

	// Release is BumpPatch, BumpMinor, BumpMajor or BumpNoRelease.
	Release Bump
}

// Plugin is one release capability with its options. Exactly one field
// is set.
type Plugin struct {
	Notes  *NotesPlugin
	Forge  *ForgePlugin
	Commit *CommitPlugin
	NPM    *NPMPlugin
	Orb    *OrbPlugin
	Gem    *GemPlugin
	Python *PythonPlugin
	Exec   *ExecHooks
}

// NotesPlugin renders the release notes.
type NotesPlugin struct{}

// ForgeGitHub is the Provider of the GitHub forge.
const ForgeGitHub = "github"

// ForgePlugin publishes a release on a Git forge.
type ForgePlugin struct {
	// Provider names the forge implementation, such as ForgeGitHub.
	Provider string

	// Assets are globs of the files to upload to the release.
	Assets []string
}

// CommitPlugin makes a release commit.
type CommitPlugin struct {
	// Files are globs of the files to commit.
	Files []string
}

// NPMPlugin publishes an npm package.
type NPMPlugin struct {
	// PkgRoot is the package folder, or empty for the repository.
	PkgRoot string
}

// OrbPlugin publishes a CircleCI orb.
type OrbPlugin struct {
	// Name is the orb name, such as "namespace/orb".
	Name string

	// Path is the orb file.
	Path string
}

// DefaultGemDir is the gem folder of a GemPlugin without a Dir: the
// Ruby client folder that stencil renders.
const DefaultGemDir = "api/clients/ruby"

// GemPlugin builds a Ruby gem and publishes it to the GitHub Packages
// RubyGems registry, as the stencil Ruby client hooks
// (shell/ruby/build.sh and shell/ruby/publish.sh) do.
type GemPlugin struct {
	// Dir is the gem folder, relative to the repository. Empty means
	// DefaultGemDir.
	Dir string

	// Name is the gem name. Empty means "<name>_client", with the name
	// in service.yaml.
	Name string

	// VersionFile is the Ruby file with the VERSION constant, relative
	// to Dir. Empty means "lib/<gem name>/version.rb".
	VersionFile string

	// Owner is the GitHub organization of the registry. Empty means the
	// owner of the origin remote.
	Owner string
}

// ExecHooks are the shell commands of an exec plugin, one for each step.
// Each command uses ${...} lookups of release fields (see
// CheckExecCommand). An empty command does nothing.
type ExecHooks struct {
	GenerateNotes string
	Prepare       string
	Publish       string
	Success       string
}

// Forge returns the first forge plugin, or nil.
func (c *Config) Forge() *ForgePlugin {
	for _, p := range c.Plugins {
		if p.Forge != nil {
			return p.Forge
		}
	}
	return nil
}
