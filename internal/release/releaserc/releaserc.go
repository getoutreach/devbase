// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Loads .releaserc.yaml into a typed model and rejects
// any plugin or option that the release command does not support.

// Package releaserc loads the subset of the semantic-release
// configuration (.releaserc.yaml) that stencil renders into spec.Config.
package releaserc

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/Masterminds/semver/v3"
	"github.com/getoutreach/devbase/v2/internal/release/internal/strictyaml"
	"github.com/getoutreach/devbase/v2/internal/release/spec"
	"go.yaml.in/yaml/v3"
)

// File is the semantic-release config file name.
const File = ".releaserc.yaml"

// majorReleaseCheckerCmd is the exec hook that stencil-base renders
// when major versions are not allowed. The release command replaces it
// with a built-in check.
const majorReleaseCheckerCmd = "./scripts/shell-wrapper.sh major-release-checker.js ${lastRelease.version} ${nextRelease.version}"

// releaseValues are the allowed values of a release rule's release key.
//
//nolint:gochecknoglobals // Why: read-only lookup table.
var releaseValues = map[spec.Bump]bool{
	spec.BumpPatch: true, spec.BumpMinor: true, spec.BumpMajor: true, spec.BumpNoRelease: true,
}

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
	l := &loader{cfg: &spec.Config{Branches: branches}}
	for i := range raw.Plugins {
		if err := l.addPlugin(&raw.Plugins[i]); err != nil {
			return nil, nil, err
		}
	}
	return l.cfg, l.warnings, nil
}

// loader holds the config and the warnings while Parse reads plugins.
type loader struct {
	cfg      *spec.Config
	warnings []string
}

// addPlugin validates one plugins entry and adds it to the config.
func (l *loader) addPlugin(n *yaml.Node) error {
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
			l.cfg.Rules = append(l.cfg.Rules, r)
		}
		return nil
	case "@semantic-release/release-notes-generator":
		if err := decodeNode(opts, &struct{}{}, name); err != nil {
			return err
		}
		l.add(spec.Plugin{Notes: &spec.NotesPlugin{}})
		return nil
	case "@semantic-release/github", "@semantic-release/git":
		var o struct {
			Assets []string `yaml:"assets"`
		}
		if err := decodeNode(opts, &o, name); err != nil {
			return err
		}
		if name == "@semantic-release/git" {
			l.add(spec.Plugin{Commit: &spec.CommitPlugin{Files: o.Assets}})
		} else {
			l.add(spec.Plugin{Forge: &spec.ForgePlugin{Provider: spec.ForgeGitHub, Assets: o.Assets}})
		}
		return nil
	case "@semantic-release/npm":
		var o struct {
			PkgRoot string `yaml:"pkgRoot"`
		}
		if err := decodeNode(opts, &o, name); err != nil {
			return err
		}
		l.add(spec.Plugin{NPM: &spec.NPMPlugin{PkgRoot: o.PkgRoot}})
		return nil
	case "@getoutreach/semantic-release-circleci-orb":
		var o struct {
			OrbName string `yaml:"orbName"`
			OrbPath string `yaml:"orbPath"`
		}
		if err := decodeNode(opts, &o, name); err != nil {
			return err
		}
		if o.OrbName == "" {
			return fmt.Errorf("%w: %s needs orbName", spec.ErrUnsupported, name)
		}
		l.add(spec.Plugin{Orb: &spec.OrbPlugin{Name: o.OrbName, Path: o.OrbPath}})
		return nil
	case "@semantic-release/exec":
		return l.addExec(opts, name)
	}
	return fmt.Errorf("%w: plugin %q", spec.ErrUnsupported, name)
}

// add appends a plugin to the config.
func (l *loader) add(p spec.Plugin) {
	l.cfg.Plugins = append(l.cfg.Plugins, p)
}

// addExec adds an exec plugin. The major-release-checker.js hook becomes
// the built-in major version check, and an exec plugin with no other
// command is left out.
func (l *loader) addExec(opts *yaml.Node, name string) error {
	var h execHook
	if err := decodeNode(opts, &h, name); err != nil {
		return err
	}
	if strings.TrimSpace(h.GenerateNotes) == majorReleaseCheckerCmd {
		l.cfg.MajorCheck = true
		h.GenerateNotes = ""
	}
	for _, cmd := range []string{h.GenerateNotes, h.Prepare, h.Publish, h.Success} {
		if err := l.validateTemplate(cmd); err != nil {
			return err
		}
	}
	if hooks := spec.ExecHooks(h); hooks != (spec.ExecHooks{}) {
		l.add(spec.Plugin{Exec: &hooks})
	}
	return nil
}

// validateTemplate checks an exec command and keeps its warnings.
func (l *loader) validateTemplate(cmd string) error {
	warnings, err := spec.CheckExecCommand(cmd)
	l.warnings = append(l.warnings, warnings...)
	return err
}

// maintenanceRangeRegex matches a maintenance branch name such as
// "1.x" or "1.2.x", as semantic-release's isMaintenanceRange does.
var maintenanceRangeRegex = regexp.MustCompile(`(?i)^\d+(?:\.(?:\d+|x))?\.x$`)

// parseBranches validates the branches list as semantic-release does.
// The release command supports one release branch and any number of
// prerelease branches; maintenance branches and more release branches
// need range logic that it does not have. The release branch gets the
// default channel and every prerelease branch is its own channel.
func parseBranches(nodes []yaml.Node) ([]spec.Branch, error) {
	out := make([]spec.Branch, 0, len(nodes))
	names, prereleases := map[string]bool{}, map[string]bool{}
	releaseBranches := 0
	for i := range nodes {
		b, err := parseBranch(&nodes[i])
		if err != nil {
			return nil, err
		}
		if names[b.Name] {
			return nil, fmt.Errorf("%w: duplicate branch %q", spec.ErrUnsupported, b.Name)
		}
		names[b.Name] = true
		if b.IsPrerelease() {
			if prereleases[b.Prerelease] {
				return nil, fmt.Errorf("%w: two branches with prerelease %q", spec.ErrUnsupported, b.Prerelease)
			}
			prereleases[b.Prerelease] = true
		} else {
			releaseBranches++
		}
		out = append(out, b)
	}
	if releaseBranches != 1 {
		return nil, fmt.Errorf("%w: %d release branches (exactly one is supported)", spec.ErrUnsupported, releaseBranches)
	}
	return out, nil
}

// parseBranch validates one branches entry, which is a name or an object.
func parseBranch(n *yaml.Node) (spec.Branch, error) {
	var rb rawBranch
	if n.Kind == yaml.ScalarNode {
		rb.Name = n.Value
	} else if err := decodeNode(n, &rb, "branches"); err != nil {
		return spec.Branch{}, err
	}
	if !validBranchName(rb.Name) {
		return spec.Branch{}, fmt.Errorf("%w: invalid branch name %q", spec.ErrUnsupported, rb.Name)
	}
	if maintenanceRangeRegex.MatchString(rb.Name) {
		return spec.Branch{}, fmt.Errorf("%w: maintenance branch %q", spec.ErrUnsupported, rb.Name)
	}
	b := spec.Branch{Name: rb.Name}
	p := &rb.Prerelease
	switch {
	case p.Kind == 0 || p.Tag == "!!null" || (p.Tag == "!!bool" && p.Value == "false"):
		return b, nil
	case p.Tag == "!!bool" && p.Value == "true":
		b.Prerelease = rb.Name
	case p.Tag == "!!str":
		b.Prerelease = p.Value
	default:
		return spec.Branch{}, fmt.Errorf("%w: branch %q prerelease %q", spec.ErrUnsupported, rb.Name, p.Value)
	}
	// As semantic-release does, check that 1.0.0-<id>.1 is valid. This
	// also rejects an empty identifier.
	if _, err := semver.StrictNewVersion("1.0.0-" + b.Prerelease + ".1"); err != nil {
		return spec.Branch{}, fmt.Errorf("%w: branch %q prerelease identifier %q", spec.ErrUnsupported, rb.Name, b.Prerelease)
	}
	// semantic-release compares the id with each part of a version's
	// prerelease, and node-semver stores a numeric part as a number, so
	// it never finds the last release of a branch with a numeric or
	// dotted id.
	if strings.Contains(b.Prerelease, ".") || strings.Trim(b.Prerelease, "0123456789") == "" {
		return spec.Branch{}, fmt.Errorf("%w: branch %q prerelease identifier %q never matches its tags",
			spec.ErrUnsupported, rb.Name, b.Prerelease)
	}
	b.Channel = rb.Name
	return b, nil
}

// validBranchName reports whether "refs/heads/<name>" passes
// git check-ref-format, which semantic-release runs on each branch.
func validBranchName(name string) bool {
	if name == "" || name == "@" || strings.HasSuffix(name, "/") || strings.HasSuffix(name, ".") ||
		strings.Contains(name, "..") || strings.Contains(name, "@{") || strings.Contains(name, "//") {
		return false
	}
	for _, c := range name {
		if c < 0x20 || c == 0x7f || strings.ContainsRune(" ~^:?*[\\", c) {
			return false
		}
	}
	for comp := range strings.SplitSeq(name, "/") {
		if comp == "" || strings.HasPrefix(comp, ".") || strings.HasSuffix(comp, ".lock") {
			return false
		}
	}
	return true
}

// parseRule validates one release rule. A null rule, a rule without a
// valid release, and a glob that is empty or uses micromatch syntax that
// the release command does not match the same way are errors.
func parseRule(n *yaml.Node) (spec.Rule, error) {
	if n.Kind != yaml.MappingNode {
		return spec.Rule{}, fmt.Errorf("%w: release rule at line %d is not an object", spec.ErrUnsupported, n.Line)
	}
	var r rule
	if err := decodeNode(n, &r, "releaseRules"); err != nil {
		return spec.Rule{}, err
	}
	if !releaseValues[r.Release] {
		return spec.Rule{}, fmt.Errorf("%w: release rule with release %q", spec.ErrUnsupported, r.Release)
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		key, value := n.Content[i].Value, n.Content[i+1].Value
		if key == "type" || key == "scope" || key == "subject" {
			if err := spec.ValidateGlob(value); err != nil {
				return spec.Rule{}, fmt.Errorf("release rule %s %q: %w", key, value, err)
			}
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
