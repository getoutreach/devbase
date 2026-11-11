// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Loads .releaserc.yaml into a typed model and rejects
// any plugin or option that the release command does not support.

// Package releaserc loads the subset of the semantic-release
// configuration (.releaserc.yaml) that stencil renders into spec.Config.
package releaserc

import (
	"fmt"
	"strings"

	"github.com/getoutreach/devbase/v2/internal/release/internal/strictyaml"
	"github.com/getoutreach/devbase/v2/internal/release/spec"
	"go.yaml.in/yaml/v3"
)

// File is the semantic-release config file name.
const File = ".releaserc.yaml"

// These are the exec hooks that stencil-golang renders to build the
// GoReleaser archives. The build command replaces them.
const (
	// buildCmd is the hook before the release:build mise task.
	buildCmd = "make release APP_VERSION=${nextRelease.version}"

	// buildTaskCmd is the hook that runs the release:build mise task.
	buildTaskCmd = "APP_VERSION=${nextRelease.version} mise run --quiet release:build 1>&2"
)

// majorReleaseCheckerCmd is the exec hook that stencil-base renders
// when major versions are not allowed. The release command replaces it
// with a built-in check.
const majorReleaseCheckerCmd = "./scripts/shell-wrapper.sh major-release-checker.js ${lastRelease.version} ${nextRelease.version}"

// rule is the YAML form of a commit-analyzer releaseRules entry.
type rule struct {
	Type     string    `yaml:"type"`
	Scope    string    `yaml:"scope"`
	Subject  string    `yaml:"subject"`
	Breaking bool      `yaml:"breaking"`
	Revert   bool      `yaml:"revert"`
	Release  spec.Bump `yaml:"release"`
}

// rawConfig is the YAML form of .releaserc.yaml.
type rawConfig struct {
	Preset   string      `yaml:"preset"`
	Branches []yaml.Node `yaml:"branches"`
	Plugins  []yaml.Node `yaml:"plugins"`
}

// rawBranch is the object form of a branches entry. A key that is not
// here, such as channel or range, is rejected.
type rawBranch struct {
	Name       string    `yaml:"name"`
	Prerelease yaml.Node `yaml:"prerelease"`
}

// execHook is the exec plugin options.
type execHook struct {
	GenerateNotes string `yaml:"generateNotesCmd"`
	Prepare       string `yaml:"prepareCmd"`
	Publish       string `yaml:"publishCmd"`
	Success       string `yaml:"successCmd"`
}

// Parse parses and validates the contents of a .releaserc.yaml file and
// maps it into the release model. Each plugin identifier selects a
// built-in capability, in plugin order. It also returns warnings: problems
// that do not stop a release.
func Parse(b []byte) (*spec.Config, []string, error) {
	cfg, warnings, err := parse(b)
	if err != nil {
		return nil, nil, fmt.Errorf("%s: %w", File, err)
	}
	return cfg, warnings, nil
}

// parse is Parse without the file name in errors.
func parse(b []byte) (*spec.Config, []string, error) {
	var raw rawConfig
	if err := strictyaml.Decode(b, &raw); err != nil {
		return nil, nil, fmt.Errorf("%w: %w", spec.ErrUnsupported, err)
	}
	if raw.Preset != "conventionalcommits" {
		return nil, nil, fmt.Errorf("%w: preset %q (only conventionalcommits)", spec.ErrUnsupported, raw.Preset)
	}

	branches, err := parseBranches(raw.Branches)
	if err != nil {
		return nil, nil, err
	}
	cfg := &spec.Config{Branches: branches}
	for i := range raw.Plugins {
		if err := addPlugin(cfg, &raw.Plugins[i]); err != nil {
			return nil, nil, err
		}
	}
	warnings, err := cfg.Validate()
	if err != nil {
		return nil, nil, err
	}
	return cfg, warnings, nil
}

// addPlugin validates one plugins entry and adds it to the config.
func addPlugin(cfg *spec.Config, n *yaml.Node) error {
	name, opts, err := splitPlugin(n)
	if err != nil {
		return err
	}

	switch name {
	case "@semantic-release/commit-analyzer":
		var o struct {
			ReleaseRules []yaml.Node `yaml:"releaseRules"`
		}
		if err := decodeNode(opts, &o, name); err != nil {
			return err
		}
		for i := range o.ReleaseRules {
			r, err := parseRule(&o.ReleaseRules[i])
			if err != nil {
				return err
			}
			cfg.Rules = append(cfg.Rules, r)
		}
		return nil
	case "@semantic-release/release-notes-generator":
		if err := decodeNode(opts, &struct{}{}, name); err != nil {
			return err
		}
		cfg.Plugins = append(cfg.Plugins, spec.Plugin{Notes: &spec.NotesPlugin{}})
		return nil
	case "@semantic-release/github", "@semantic-release/git":
		var o struct {
			Assets []string `yaml:"assets"`
		}
		if err := decodeNode(opts, &o, name); err != nil {
			return err
		}
		if name == "@semantic-release/git" {
			cfg.Plugins = append(cfg.Plugins, spec.Plugin{Commit: &spec.CommitPlugin{Files: o.Assets}})
		} else {
			cfg.Plugins = append(cfg.Plugins, spec.Plugin{Forge: &spec.ForgePlugin{Provider: spec.ForgeGitHub, Assets: o.Assets}})
		}
		return nil
	case "@semantic-release/npm":
		var o struct {
			PkgRoot string `yaml:"pkgRoot"`
		}
		if err := decodeNode(opts, &o, name); err != nil {
			return err
		}
		cfg.Plugins = append(cfg.Plugins, spec.Plugin{NPM: &spec.NPMPlugin{PkgRoot: o.PkgRoot}})
		return nil
	case "@getoutreach/semantic-release-circleci-orb":
		var o struct {
			OrbName string `yaml:"orbName"`
			OrbPath string `yaml:"orbPath"`
		}
		if err := decodeNode(opts, &o, name); err != nil {
			return err
		}
		cfg.Plugins = append(cfg.Plugins, spec.Plugin{Orb: &spec.OrbPlugin{Name: o.OrbName, Path: o.OrbPath}})
		return nil
	case "@semantic-release/exec":
		return addExec(cfg, opts, name)
	}
	return fmt.Errorf("%w: plugin %q", spec.ErrUnsupported, name)
}

// addExec adds an exec plugin. The major-release-checker.js hook becomes
// the built-in major version check, the build hook becomes the build
// command, and an exec plugin with no other command is left out.
func addExec(cfg *spec.Config, opts *yaml.Node, name string) error {
	var h execHook
	if err := decodeNode(opts, &h, name); err != nil {
		return err
	}
	switch strings.TrimSpace(h.GenerateNotes) {
	case majorReleaseCheckerCmd:
		cfg.MajorCheck = true
		h.GenerateNotes = ""
	case buildCmd, buildTaskCmd:
		cfg.Build = true
		h.GenerateNotes = ""
	}
	// Another form of the build hook would run as a plain hook and skip
	// the build command, so it fails closed.
	for _, cmd := range []string{h.GenerateNotes, h.Prepare, h.Publish, h.Success} {
		if strings.Contains(cmd, "make release") || strings.Contains(cmd, "release:build") {
			return fmt.Errorf("%w: build hook %q; use %q", spec.ErrUnsupported, cmd, buildTaskCmd)
		}
	}
	if hooks := spec.ExecHooks(h); hooks != (spec.ExecHooks{}) {
		cfg.Plugins = append(cfg.Plugins, spec.Plugin{Exec: &hooks})
	}
	return nil
}

// parseBranches maps the branches list. The release branch gets the
// default channel and every prerelease branch is its own channel, as in
// semantic-release. spec.Config.Validate checks the list.
func parseBranches(nodes []yaml.Node) ([]spec.Branch, error) {
	out := make([]spec.Branch, 0, len(nodes))
	for i := range nodes {
		b, err := parseBranch(&nodes[i])
		if err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, nil
}

// parseBranch maps one branches entry, which is a name or an object.
func parseBranch(n *yaml.Node) (spec.Branch, error) {
	var rb rawBranch
	if n.Kind == yaml.ScalarNode {
		rb.Name = n.Value
	} else if err := decodeNode(n, &rb, "branches"); err != nil {
		return spec.Branch{}, err
	}
	b := spec.Branch{Name: rb.Name}
	p := &rb.Prerelease
	switch {
	case p.Kind == 0 || p.Tag == "!!null" || (p.Tag == "!!bool" && p.Value == "false"):
		return b, nil
	case p.Tag == "!!bool" && p.Value == "true":
		b.Prerelease = rb.Name
	case p.Tag == "!!str" && p.Value != "":
		b.Prerelease = p.Value
	default:
		return spec.Branch{}, fmt.Errorf("%w: branch %q prerelease %q", spec.ErrUnsupported, rb.Name, p.Value)
	}
	b.Channel = rb.Name
	return b, nil
}

// parseRule maps one release rule. A null rule and an empty glob are
// errors; spec.Config.Validate checks the rest.
func parseRule(n *yaml.Node) (spec.Rule, error) {
	if n.Kind != yaml.MappingNode {
		return spec.Rule{}, fmt.Errorf("%w: release rule at line %d is not an object", spec.ErrUnsupported, n.Line)
	}
	var r rule
	if err := decodeNode(n, &r, "releaseRules"); err != nil {
		return spec.Rule{}, err
	}
	// An empty glob makes micromatch throw. The model cannot tell it from
	// a missing key, so the loader checks the keys.
	for i := 0; i+1 < len(n.Content); i += 2 {
		key, value := n.Content[i].Value, n.Content[i+1].Value
		if (key == "type" || key == "scope" || key == "subject") && value == "" {
			return spec.Rule{}, fmt.Errorf("%w: release rule with an empty %s", spec.ErrUnsupported, key)
		}
	}
	return spec.Rule(r), nil
}

// splitPlugin returns the identifier and options node of a plugins
// entry, which is either a string or a [name, options] list.
func splitPlugin(n *yaml.Node) (string, *yaml.Node, error) {
	switch {
	case n.Kind == yaml.ScalarNode:
		return n.Value, nil, nil
	case n.Kind == yaml.SequenceNode && len(n.Content) == 1 && n.Content[0].Kind == yaml.ScalarNode:
		return n.Content[0].Value, nil, nil
	case n.Kind == yaml.SequenceNode && len(n.Content) == 2 && n.Content[0].Kind == yaml.ScalarNode:
		return n.Content[0].Value, n.Content[1], nil
	}
	return "", nil, fmt.Errorf("%w: plugin entry at line %d", spec.ErrUnsupported, n.Line)
}

// decodeNode decodes a YAML node into v, rejects unknown keys, and
// wraps a failure in spec.ErrUnsupported.
func decodeNode(n *yaml.Node, v any, what string) error {
	if err := strictyaml.DecodeNode(n, v); err != nil {
		return fmt.Errorf("%w: %s: %w", spec.ErrUnsupported, what, err)
	}
	return nil
}
