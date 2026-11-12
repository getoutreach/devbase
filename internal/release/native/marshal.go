// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Writes a release model as the release section of
// scripts/devbase.yaml, without the values that are the defaults.

package native

import (
	"bytes"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strconv"

	"github.com/getoutreach/devbase/v2/internal/release/spec"
	"go.yaml.in/yaml/v3"
)

// ErrNotEquivalent is returned by Marshal when the section that it wrote
// does not load as the same configuration. It is a bug in this package.
var ErrNotEquivalent = errors.New("the release section does not load as the same configuration")

// Marshal returns cfg as a scripts/devbase.yaml document with only the
// release section. It leaves out the values that are the defaults. It
// loads the document again and returns an error when the result is not
// cfg, so that the document is always equivalent to cfg. An empty list
// is equivalent to no list, except for the files of a release commit.
// The units of cfg are as Parse returns them, without the defaults and
// the go.mod values that Repo.SetUnits adds.
func Marshal(cfg *spec.Config) ([]byte, error) {
	s, err := fromConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", Key, err)
	}
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(map[string]*section{Key: s}); err != nil {
		return nil, fmt.Errorf("encode %s: %w", Key, err)
	}
	if err := enc.Close(); err != nil {
		return nil, fmt.Errorf("encode %s: %w", Key, err)
	}
	got, _, err := Parse(buf.Bytes())
	if err != nil {
		return nil, err
	}
	if !reflect.DeepEqual(got, normalize(cfg)) {
		return nil, ErrNotEquivalent
	}
	return buf.Bytes(), nil
}

// fromConfig returns the section of cfg without the defaults.
func fromConfig(cfg *spec.Config) (*section, error) {
	s := &section{AllowMajorVersions: !cfg.MajorCheck}
	for i := range cfg.Branches {
		b, err := fromBranch(&cfg.Branches[i])
		if err != nil {
			return nil, err
		}
		s.Branches = append(s.Branches, b)
	}
	if !reflect.DeepEqual(emptyToNil(cfg.Rules), defaultRuleList(s.AllowMajorVersions)) {
		rules := []rule{}
		for _, r := range cfg.Rules {
			rules = append(rules, fromRule(r))
		}
		s.Rules = &rules
	}
	if cfg.Build != (cfg.Artifacts != nil) {
		return nil, fmt.Errorf("%w: a build without artifacts, or artifacts without a build", spec.ErrUnsupported)
	}
	if cfg.Artifacts != nil {
		s.Build = fromArtifacts(cfg.Artifacts)
	}
	defaults, err := plugins(defaultPublisherList(), cfg.Artifacts)
	if err != nil {
		return nil, err
	}
	s.Root, s.Units = fromUnits(cfg)
	if !reflect.DeepEqual(normalize(cfg).Plugins, defaults) {
		publishers := make([]publisher, 0, len(cfg.Plugins))
		for i := range cfg.Plugins {
			publishers = append(publishers, fromPlugin(&cfg.Plugins[i], cfg.Artifacts))
		}
		s.Publishers = &publishers
	}
	return s, nil
}

// fromBranch maps a branch. The channel of a branch is not in the
// section, so it must be the channel that the section gives.
func fromBranch(b *spec.Branch) (branch, error) {
	out := branch{Name: b.Name}
	want := ""
	if b.IsPrerelease() {
		out.Prerelease = &b.Prerelease
		want = b.Name
	}
	if b.Channel != want {
		return branch{}, fmt.Errorf("%w: branch %q with channel %q", spec.ErrUnsupported, b.Name, b.Channel)
	}
	return out, nil
}

// fromRule maps a rule.
func fromRule(r spec.Rule) rule {
	out := rule{Breaking: r.Breaking, Revert: r.Revert, Release: string(r.Release)}
	if r.Release == spec.BumpNoRelease {
		out.Release = releaseNone
	}
	for _, f := range []struct {
		in  string
		out **string
	}{{r.Type, &out.Type}, {r.Scope, &out.Scope}, {r.Subject, &out.Subject}} {
		if f.in != "" {
			*f.out = &f.in
		}
	}
	return out
}

// fromArtifacts maps the artifacts without the defaults.
func fromArtifacts(a *spec.Artifacts) *build {
	b := &build{Before: listOrDefault(a.Hooks, defaultBefore)}
	if a.Checksums != defaultChecksums {
		b.Checksums = a.Checksums
	}
	targets := make([]string, 0, len(a.Targets))
	for i := range a.Targets {
		targets = append(targets, a.Targets[i].String())
	}
	b.Targets = listOrDefault(targets, defaultTargets)
	for i := range a.Builds {
		in := &a.Builds[i]
		g := goBuild{
			Binary:  in.Binary,
			LDFlags: listOrDefault(in.LDFlags, defaultLDFlags),
			Env:     listOrDefault(in.Env, defaultEnv),
		}
		if in.ID != in.Binary {
			g.ID = in.ID
		}
		if in.Main != defaultMainDir+in.Binary {
			g.Main = in.Main
		}
		b.Builds = append(b.Builds, g)
	}
	if !slices.Equal(a.Files, defaultFiles()) {
		files := make([]archiveFile, 0, len(a.Files))
		for _, f := range a.Files {
			files = append(files, archiveFile{Src: f.Src, Dst: f.Dst, Mode: f.Mode})
		}
		b.Archive = &archive{Files: &files}
	}
	return b
}

// fromPlugin maps a plugin. a is the artifacts of the release, which set
// the default assets of the GitHub release.
func fromPlugin(pl *spec.Plugin, a *spec.Artifacts) publisher {
	switch {
	case pl.Notes != nil:
		return publisher{Notes: &struct{}{}}
	case pl.Forge != nil:
		return publisher{GitHub: &github{Assets: listOrDefault(pl.Forge.Assets, defaultAssets(a))}}
	case pl.Commit != nil:
		c := &commit{}
		if pl.Commit.Files != nil {
			files := slices.Clone(pl.Commit.Files)
			c.Files = &files
		}
		return publisher{Commit: c}
	case pl.NPM != nil:
		return publisher{NPM: &npm{PackageRoot: pl.NPM.PkgRoot}}
	case pl.Orb != nil:
		return publisher{Orb: &orb{Name: pl.Orb.Name, Path: pl.Orb.Path}}
	case pl.Gem != nil:
		g := gem(*pl.Gem)
		if g.Dir == spec.DefaultGemDir {
			g.Dir = ""
		}
		return publisher{Gem: &g}
	case pl.Python != nil:
		py := python(*pl.Python)
		if py.Dir == spec.DefaultPythonDir {
			py.Dir = ""
		}
		return publisher{Python: &py}
	case pl.Exec != nil:
		return publisher{Hooks: &hooks{
			GenerateNotes: pl.Exec.GenerateNotes, Prepare: pl.Exec.Prepare, Publish: pl.Exec.Publish, Success: pl.Exec.Success,
		}}
	}
	return publisher{}
}

// listOrDefault returns nil when list is def, and else a copy of list. An
// empty list is not nil, so that it replaces a default that is not empty.
func listOrDefault(list, def []string) *[]string {
	if slices.Equal(list, def) {
		return nil
	}
	out := append([]string{}, list...)
	return &out
}

// normalize returns a copy of cfg with the empty lists that the section
// cannot tell from no list set to nil, and the client plugins with their
// defaults, as Parse returns them.
func normalize(cfg *spec.Config) *spec.Config {
	out := *cfg
	out.Rules = emptyToNil(cfg.Rules)
	out.Plugins = nil
	for _, pl := range cfg.Plugins {
		switch {
		case pl.Forge != nil:
			f := *pl.Forge
			f.Assets = emptyToNil(f.Assets)
			pl.Forge = &f
		case pl.Gem != nil:
			g := *pl.Gem
			g.Dir = g.GemDir()
			pl.Gem = &g
		case pl.Python != nil:
			py := pl.Python.WithDefaults()
			pl.Python = &py
		}
		out.Plugins = append(out.Plugins, pl)
	}
	out.RootPaths = spec.RootPaths{Exclude: emptyToNil(cfg.RootPaths.Exclude), Shared: emptyToNil(cfg.RootPaths.Shared)}
	out.Units = nil
	for _, u := range cfg.Units {
		u.Paths, u.Exclude = emptyToNil(u.Paths), emptyToNil(u.Exclude)
		out.Units = append(out.Units, u)
	}
	if cfg.Artifacts != nil {
		a := *cfg.Artifacts
		a.Hooks = emptyToNil(a.Hooks)
		a.Targets = emptyToNil(a.Targets)
		a.Files = emptyToNil(a.Files)
		a.Builds = nil
		for _, b := range cfg.Artifacts.Builds {
			b.LDFlags, b.Env = emptyToNil(b.LDFlags), emptyToNil(b.Env)
			a.Builds = append(a.Builds, b)
		}
		out.Artifacts = &a
	}
	return &out
}

// emptyToNil returns s, or nil when s is empty.
func emptyToNil[T any](s []T) []T {
	if len(s) == 0 {
		return nil
	}
	return s
}

// MarshalYAML encodes the publisher as its name when it has no options,
// and else as a map with one key from the name to the options.
func (p publisher) MarshalYAML() (any, error) {
	name, opts := p.nameAndOptions()
	if opts == nil {
		return name, nil
	}
	return map[string]any{name: opts}, nil
}

// nameAndOptions returns the name of the publisher and its options, or
// nil options when they are all zero.
func (p *publisher) nameAndOptions() (name string, opts any) {
	switch {
	case p.Notes != nil:
		return publisherNotes, nil
	case p.GitHub != nil:
		return publisherGitHub, nilIfZero(p.GitHub, &github{})
	case p.Commit != nil:
		return publisherCommit, nilIfZero(p.Commit, &commit{})
	case p.NPM != nil:
		return publisherNPM, nilIfZero(p.NPM, &npm{})
	case p.Orb != nil:
		return publisherOrb, p.Orb
	case p.Gem != nil:
		return publisherGem, nilIfZero(p.Gem, &gem{})
	case p.Python != nil:
		return publisherPython, nilIfZero(p.Python, &python{})
	}
	return publisherHooks, p.Hooks
}

// nilIfZero returns nil when *v is *zero, and else v.
func nilIfZero[T comparable](v, zero *T) any {
	if *v == *zero {
		return nil
	}
	return v
}

// MarshalYAML encodes the archive file with its mode in octal, such as
// 0755.
func (f archiveFile) MarshalYAML() (any, error) {
	out := struct {
		Src  string     `yaml:"src"`
		Dst  string     `yaml:"dst,omitempty"`
		Mode *yaml.Node `yaml:"mode,omitempty"`
	}{Src: f.Src, Dst: f.Dst}
	if f.Mode != 0 {
		out.Mode = &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!int", Value: "0" + strconv.FormatUint(uint64(f.Mode), 8)}
	}
	return out, nil
}
