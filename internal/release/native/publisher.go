// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: The publishers of the release section: a name, or a map
// with one key from the name to the options.

package native

import (
	"errors"
	"fmt"
	"slices"

	"github.com/getoutreach/devbase/v2/internal/release/spec"
	"go.yaml.in/yaml/v3"
)

// These are the publisher names.
const (
	// publisherNotes renders the release notes.
	publisherNotes = "notes"

	// publisherGitHub makes the GitHub release.
	publisherGitHub = "github"

	// publisherCommit makes the release commit.
	publisherCommit = "commit"

	// publisherNPM publishes an npm package.
	publisherNPM = "npm"

	// publisherOrb publishes a CircleCI orb.
	publisherOrb = "orb"

	// publisherGem publishes the Ruby client gem.
	publisherGem = "gem"

	// publisherPython publishes the Python client.
	publisherPython = "python"

	// publisherHooks runs shell commands.
	publisherHooks = "hooks"
)

// These are the errors of a publishers entry. The loader wraps them in
// spec.ErrUnsupported.
var (
	// errNotPublisher is returned for an entry that is not a name or a
	// map with one key.
	errNotPublisher = errors.New("a publisher is a name, or a map with one key from the name to its options")

	// errUnknownPublisher is returned for an unknown publisher name.
	errUnknownPublisher = errors.New("unknown publisher (one of notes, github, commit, npm, orb, gem, python, hooks)")

	// errNotMap is returned for options that are not a map.
	errNotMap = errors.New("the options are not a map")

	// errUnknownKey is returned for an unknown option key.
	errUnknownKey = errors.New("unknown key")
)

// publisher is one publishers entry. Exactly one field is set.
type publisher struct {
	Notes  *struct{}
	GitHub *github
	Commit *commit
	NPM    *npm
	Orb    *orb
	Gem    *gem
	Python *python
	Hooks  *hooks
}

// github is the GitHub release.
type github struct {
	Assets *[]string `yaml:"assets,omitempty"`
}

// commit is the release commit.
type commit struct {
	Files *[]string `yaml:"files,omitempty"`
}

// npm is the npm package.
type npm struct {
	PackageRoot string `yaml:"packageRoot,omitempty"`
}

// orb is the CircleCI orb.
type orb struct {
	Name string `yaml:"name"`
	Path string `yaml:"path,omitempty"`
}

// gem is the Ruby client gem. The fields are those of spec.GemPlugin.
type gem struct {
	Dir         string `yaml:"dir,omitempty"`
	Name        string `yaml:"name,omitempty"`
	VersionFile string `yaml:"versionFile,omitempty"`
}

// python is the Python client. The fields are those of
// spec.PythonPlugin.
type python struct {
	Dir        string `yaml:"dir,omitempty"`
	Domain     string `yaml:"domain,omitempty"`
	Account    string `yaml:"account,omitempty"`
	Region     string `yaml:"region,omitempty"`
	Repository string `yaml:"repository,omitempty"`
}

// hooks are shell commands, one for each step of the release.
type hooks struct {
	GenerateNotes string `yaml:"generateNotes,omitempty"`
	Prepare       string `yaml:"prepare,omitempty"`
	Publish       string `yaml:"publish,omitempty"`
	Success       string `yaml:"success,omitempty"`
}

// optionKeys are the option keys of each publisher.
//
//nolint:gochecknoglobals // Why: read-only lookup table.
var optionKeys = map[string][]string{
	publisherNotes:  nil,
	publisherGitHub: {"assets"},
	publisherCommit: {"files"},
	publisherNPM:    {"packageRoot"},
	publisherOrb:    {"name", "path"},
	publisherGem:    {"dir", "name", "versionFile"},
	publisherPython: {"dir", "domain", "account", "region", "repository"},
	publisherHooks:  {"generateNotes", "prepare", "publish", "success"},
}

// defaultPublisherList returns the default publishers: the notes and
// the GitHub release.
func defaultPublisherList() []publisher {
	return []publisher{{Notes: &struct{}{}}, {GitHub: &github{}}}
}

// UnmarshalYAML decodes a publisher name, or a map with one key from the
// name to its options. It checks the option keys itself, because a
// yaml.Node decodes without the KnownFields check, and so the errors keep
// their line numbers.
func (p *publisher) UnmarshalYAML(n *yaml.Node) error {
	var name *yaml.Node
	var opts *yaml.Node
	switch {
	case n.Kind == yaml.ScalarNode:
		name = n
	case n.Kind == yaml.MappingNode && len(n.Content) == 2:
		name, opts = n.Content[0], n.Content[1]
	default:
		return fmt.Errorf("line %d: %w", n.Line, errNotPublisher)
	}
	if err := p.set(name.Value, opts); err != nil {
		return fmt.Errorf("line %d: publisher %q: %w", name.Line, name.Value, err)
	}
	return nil
}

// set sets the field of the publisher name to its options, or to the
// zero options when opts is nil or null.
func (p *publisher) set(name string, opts *yaml.Node) error {
	keys, ok := optionKeys[name]
	if !ok {
		return errUnknownPublisher
	}
	if opts != nil && opts.ShortTag() == "!!null" {
		opts = nil
	}
	if opts != nil {
		if err := checkKeys(opts, keys); err != nil {
			return err
		}
	}
	var v any
	switch name {
	case publisherNotes:
		p.Notes = &struct{}{}
		v = p.Notes
	case publisherGitHub:
		p.GitHub = &github{}
		v = p.GitHub
	case publisherCommit:
		p.Commit = &commit{}
		v = p.Commit
	case publisherNPM:
		p.NPM = &npm{}
		v = p.NPM
	case publisherOrb:
		p.Orb = &orb{}
		v = p.Orb
	case publisherGem:
		p.Gem = &gem{}
		v = p.Gem
	case publisherPython:
		p.Python = &python{}
		v = p.Python
	case publisherHooks:
		p.Hooks = &hooks{}
		v = p.Hooks
	}
	if opts == nil {
		return nil
	}
	return opts.Decode(v)
}

// checkKeys checks that n is a map with only the given keys.
func checkKeys(n *yaml.Node, keys []string) error {
	if n.Kind != yaml.MappingNode {
		return fmt.Errorf("line %d: %w", n.Line, errNotMap)
	}
	for i := 0; i < len(n.Content); i += 2 {
		if k := n.Content[i]; !slices.Contains(keys, k.Value) {
			return fmt.Errorf("line %d: %w %q", k.Line, errUnknownKey, k.Value)
		}
	}
	return nil
}

// plugin maps the publisher into the release model. a is the artifacts of
// the release, which set the default assets of the GitHub release.
func (p *publisher) plugin(a *spec.Artifacts) (spec.Plugin, error) {
	switch {
	case p.Notes != nil:
		return spec.Plugin{Notes: &spec.NotesPlugin{}}, nil
	case p.GitHub != nil:
		assets := defaultAssets(a)
		if p.GitHub.Assets != nil {
			assets = orDefault(p.GitHub.Assets, nil)
		}
		return spec.Plugin{Forge: &spec.ForgePlugin{Provider: spec.ForgeGitHub, Assets: assets}}, nil
	case p.Commit != nil:
		var files []string
		if p.Commit.Files != nil {
			// An empty list commits nothing; no list commits the default files.
			files = append([]string{}, *p.Commit.Files...)
		}
		return spec.Plugin{Commit: &spec.CommitPlugin{Files: files}}, nil
	case p.NPM != nil:
		return spec.Plugin{NPM: &spec.NPMPlugin{PkgRoot: p.NPM.PackageRoot}}, nil
	case p.Orb != nil:
		return spec.Plugin{Orb: &spec.OrbPlugin{Name: p.Orb.Name, Path: p.Orb.Path}}, nil
	case p.Gem != nil:
		g := spec.GemPlugin(*p.Gem)
		g.Dir = g.GemDir()
		return spec.Plugin{Gem: &g}, nil
	case p.Python != nil:
		py := (*spec.PythonPlugin)(p.Python).WithDefaults()
		return spec.Plugin{Python: &py}, nil
	}
	h := p.Hooks
	if *h == (hooks{}) {
		return spec.Plugin{}, fmt.Errorf("%w: hooks without a command", spec.ErrUnsupported)
	}
	for _, cmd := range []string{h.GenerateNotes, h.Prepare, h.Publish, h.Success} {
		if err := checkHookCommand(cmd); err != nil {
			return spec.Plugin{}, err
		}
	}
	return spec.Plugin{Exec: &spec.ExecHooks{
		GenerateNotes: h.GenerateNotes, Prepare: h.Prepare, Publish: h.Publish, Success: h.Success,
	}}, nil
}
