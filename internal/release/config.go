// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Loads .releaserc.yaml into a typed release model and
// rejects any plugin or option that devbase release does not support.

package release

import (
	"bytes"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"go.yaml.in/yaml/v3"
)

// majorReleaseCheckerCmd is the exec hook that stencil-base renders
// when major versions are not allowed. devbase release replaces it with
// a built-in check.
const majorReleaseCheckerCmd = "./scripts/shell-wrapper.sh major-release-checker.js ${lastRelease.version} ${nextRelease.version}"

// ErrUnsupportedConfig is returned for any part of .releaserc.yaml that
// devbase release does not support. The loader fails closed so that a
// repo never releases with part of its configuration ignored.
var ErrUnsupportedConfig = errors.New("unsupported release configuration")

// templateExprRegex matches a ${...} lookup in an exec command.
var templateExprRegex = regexp.MustCompile(`\$\{([^}]*)\}`)

// allowedTemplateFields are the lookups that exec commands may use.
//
//nolint:gochecknoglobals // Why: read-only lookup table.
var allowedTemplateFields = map[string]bool{
	"nextRelease.version": true,
	"nextRelease.gitTag":  true,
	"nextRelease.channel": true,
	"lastRelease.version": true,
	"lastRelease.gitTag":  true,
	"branch.name":         true,
	"options.dryRun":      true,
}

// Branch is one entry of the branches list.
type Branch struct {
	// Name is the git branch name.
	Name string `json:"name"`

	// Prerelease is the prerelease identifier (for example "rc"), or
	// empty for a release branch.
	Prerelease string `json:"prerelease,omitempty"`

	// Channel is the channel name. The first release branch uses the
	// default channel, which is the empty string.
	Channel string `json:"channel,omitempty"`
}

// IsPrerelease reports whether b is a prerelease branch.
func (b *Branch) IsPrerelease() bool {
	return b.Prerelease != ""
}

// Rule is one entry of the commit-analyzer releaseRules option.
type Rule struct {
	Type     string `yaml:"type"`
	Scope    string `yaml:"scope"`
	Subject  string `yaml:"subject"`
	Breaking bool   `yaml:"breaking"`
	Revert   bool   `yaml:"revert"`

	// Release is the release type, or "false" to stop a release.
	Release string `yaml:"release"`
}

// Config is the typed form of .releaserc.yaml. This proof of concept
// keeps only what planning needs; the other plugins are validated so
// that an unsupported option still fails closed.
type Config struct {
	// Branches are the release and prerelease branches, in order.
	Branches []Branch

	// ReleaseRules are the custom commit-analyzer rules.
	ReleaseRules []Rule

	// MajorCheck is true when major versions are blocked, which the
	// major-release-checker.js exec hook did before.
	MajorCheck bool

	// Warnings are problems that do not stop a release.
	Warnings []string
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

// ParseConfig parses and validates the contents of a .releaserc.yaml file.
func ParseConfig(b []byte) (*Config, error) {
	var raw rawConfig
	if err := decodeStrict(b, &raw, ErrUnsupportedConfig, ".releaserc.yaml"); err != nil {
		return nil, err
	}
	if raw.Preset != "conventionalcommits" {
		return nil, fmt.Errorf("%w: preset %q (only conventionalcommits)", ErrUnsupportedConfig, raw.Preset)
	}

	cfg := &Config{}
	branches, err := parseBranches(raw.Branches)
	if err != nil {
		return nil, err
	}
	cfg.Branches = branches

	for i := range raw.Plugins {
		if err := cfg.addPlugin(&raw.Plugins[i]); err != nil {
			return nil, err
		}
	}
	return cfg, nil
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

// addPlugin validates one plugins entry and keeps what planning needs.
func (c *Config) addPlugin(n *yaml.Node) error {
	name, opts, err := splitPlugin(n)
	if err != nil {
		return err
	}

	switch name {
	case "@semantic-release/commit-analyzer":
		var o struct {
			ReleaseRules []Rule `yaml:"releaseRules"`
		}
		if err := decodeNode(opts, &o, name); err != nil {
			return err
		}
		for _, r := range o.ReleaseRules {
			if r.Release != "false" && Bump(r.Release).rank() <= 0 {
				return fmt.Errorf("%w: release rule with release %q", ErrUnsupportedConfig, r.Release)
			}
		}
		c.ReleaseRules = o.ReleaseRules
		return nil
	case "@semantic-release/release-notes-generator":
		return decodeNode(opts, &struct{}{}, name)
	case "@semantic-release/github", "@semantic-release/git":
		return decodeNode(opts, &struct {
			Assets []string `yaml:"assets"`
		}{}, name)
	case "@semantic-release/npm":
		return decodeNode(opts, &struct {
			PkgRoot string `yaml:"pkgRoot"`
		}{}, name)
	case "@getoutreach/semantic-release-circleci-orb":
		var o struct {
			OrbName string `yaml:"orbName"`
			OrbPath string `yaml:"orbPath"`
		}
		if err := decodeNode(opts, &o, name); err != nil {
			return err
		}
		if o.OrbName == "" {
			return fmt.Errorf("%w: %s needs orbName", ErrUnsupportedConfig, name)
		}
		return nil
	case "@semantic-release/exec":
		var h execHook
		if err := decodeNode(opts, &h, name); err != nil {
			return err
		}
		if strings.TrimSpace(h.GenerateNotes) == majorReleaseCheckerCmd {
			c.MajorCheck = true
			h.GenerateNotes = ""
		}
		for _, cmd := range []string{h.GenerateNotes, h.Prepare, h.Publish, h.Success} {
			if err := c.validateTemplate(cmd); err != nil {
				return err
			}
		}
		return nil
	}
	return fmt.Errorf("%w: plugin %q", ErrUnsupportedConfig, name)
}

// validateTemplate rejects exec commands with lookups outside
// allowedTemplateFields, and lodash <% %> templates. The lowercase
// options.dryrun, which semantic-release always renders as empty, is
// accepted with a warning.
func (c *Config) validateTemplate(cmd string) error {
	if strings.Contains(cmd, "<%") {
		return fmt.Errorf("%w: lodash template in %q", ErrUnsupportedConfig, cmd)
	}
	for _, m := range templateExprRegex.FindAllStringSubmatch(cmd, -1) {
		field := strings.TrimSpace(m[1])
		if field == "options.dryrun" {
			c.Warnings = append(c.Warnings, "use ${options.dryRun}: semantic-release renders ${options.dryrun} as an empty string")
			continue
		}
		if !allowedTemplateFields[field] {
			return fmt.Errorf("%w: template field %q", ErrUnsupportedConfig, field)
		}
	}
	return nil
}

// parseBranches validates the branches list. Maintenance branches are
// not supported; the first release branch gets the default channel and
// every other branch is its own channel, as in semantic-release.
func parseBranches(nodes []yaml.Node) ([]Branch, error) {
	if len(nodes) == 0 {
		return nil, fmt.Errorf("%w: no branches", ErrUnsupportedConfig)
	}
	out := make([]Branch, 0, len(nodes))
	releaseBranches := 0
	for i := range nodes {
		var rb rawBranch
		n := &nodes[i]
		if n.Kind == yaml.ScalarNode {
			rb.Name = n.Value
		} else if err := decodeNode(n, &rb, "branches"); err != nil {
			return nil, err
		}
		if rb.Name == "" {
			return nil, fmt.Errorf("%w: branch with an empty name", ErrUnsupportedConfig)
		}
		b := Branch{Name: rb.Name, Channel: rb.Name}
		switch {
		case rb.Prerelease.Kind == 0:
		case rb.Prerelease.Tag == "!!bool" && rb.Prerelease.Value == "true":
			b.Prerelease = rb.Name
		case rb.Prerelease.Tag == "!!str":
			b.Prerelease = rb.Prerelease.Value
		default:
			return nil, fmt.Errorf("%w: branch %q prerelease %q", ErrUnsupportedConfig, rb.Name, rb.Prerelease.Value)
		}
		// As semantic-release does, check that 1.0.0-<id>.1 is valid.
		if _, err := ParseVersion("1.0.0-" + b.Prerelease + ".1"); b.IsPrerelease() && err != nil {
			return nil, fmt.Errorf("%w: branch %q prerelease identifier %q", ErrUnsupportedConfig, rb.Name, b.Prerelease)
		}
		if !b.IsPrerelease() {
			if releaseBranches == 0 {
				b.Channel = ""
			}
			releaseBranches++
		}
		out = append(out, b)
	}
	if releaseBranches == 0 {
		return nil, fmt.Errorf("%w: no release branch", ErrUnsupportedConfig)
	}
	return out, nil
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
	return "", nil, fmt.Errorf("%w: plugin entry at line %d", ErrUnsupportedConfig, n.Line)
}

// decodeNode decodes a YAML node into v and rejects unknown keys. A nil
// node leaves v unchanged. yaml.Node.Decode has no option for unknown
// keys, so the node is encoded and decoded again.
func decodeNode(n *yaml.Node, v any, what string) error {
	if n == nil {
		return nil
	}
	b, err := yaml.Marshal(n)
	if err != nil {
		return fmt.Errorf("re-encode %s: %w", what, err)
	}
	return decodeStrict(b, v, ErrUnsupportedConfig, what)
}

// decodeStrict decodes YAML into v and rejects unknown keys. A
// failure wraps sentinel.
func decodeStrict(b []byte, v any, sentinel error, what string) error {
	dec := yaml.NewDecoder(bytes.NewReader(b))
	dec.KnownFields(true)
	if err := dec.Decode(v); err != nil {
		return fmt.Errorf("%w: %s: %w", sentinel, what, err)
	}
	return nil
}
