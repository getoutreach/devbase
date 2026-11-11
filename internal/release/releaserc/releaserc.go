// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Loads .releaserc.yaml into a typed model and rejects
// any plugin or option that the release command does not support.

// Package releaserc loads the subset of the semantic-release
// configuration (.releaserc.yaml) that stencil renders.
package releaserc

import (
	"errors"
	"fmt"
	"path"
	"regexp"
	"strings"

	"github.com/Masterminds/semver/v3"
	"github.com/getoutreach/devbase/v2/internal/release/internal/strictyaml"
	"go.yaml.in/yaml/v3"
)

// File is the semantic-release config file name.
const File = ".releaserc.yaml"

// majorReleaseCheckerCmd is the exec hook that stencil-base renders
// when major versions are not allowed. The release command replaces it
// with a built-in check.
const majorReleaseCheckerCmd = "./scripts/shell-wrapper.sh major-release-checker.js ${lastRelease.version} ${nextRelease.version}"

// ErrUnsupported is returned for any part of .releaserc.yaml that
// the release command does not support. The loader fails closed so that a
// repo never releases with part of its configuration ignored.
var ErrUnsupported = errors.New("unsupported .releaserc.yaml configuration")

// templateExprRegex matches a ${...} lookup in an exec command.
var templateExprRegex = regexp.MustCompile(`\$\{([^}]*)\}`)

// releaseValues are the allowed values of a release rule's release key.
//
//nolint:gochecknoglobals // Why: read-only lookup table.
var releaseValues = map[string]bool{"patch": true, "minor": true, "major": true, "false": true}

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

// Parse parses and validates the contents of a .releaserc.yaml file.
func Parse(b []byte) (*Config, error) {
	var raw rawConfig
	if err := strictyaml.Decode(b, &raw); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrUnsupported, err)
	}
	if raw.Preset != "conventionalcommits" {
		return nil, fmt.Errorf("%w: preset %q (only conventionalcommits)", ErrUnsupported, raw.Preset)
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
			c.ReleaseRules = append(c.ReleaseRules, r)
		}
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
			return fmt.Errorf("%w: %s needs orbName", ErrUnsupported, name)
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
	return fmt.Errorf("%w: plugin %q", ErrUnsupported, name)
}

// validateTemplate rejects exec commands with lookups outside
// allowedTemplateFields, and lodash <% %> templates. The lowercase
// options.dryrun, which semantic-release always renders as empty, is
// accepted with a warning.
func (c *Config) validateTemplate(cmd string) error {
	if strings.Contains(cmd, "<%") {
		return fmt.Errorf("%w: lodash template in %q", ErrUnsupported, cmd)
	}
	for _, m := range templateExprRegex.FindAllStringSubmatch(cmd, -1) {
		field := strings.TrimSpace(m[1])
		if field == "options.dryrun" {
			c.Warnings = append(c.Warnings, "use ${options.dryRun}: semantic-release renders ${options.dryrun} as an empty string")
			continue
		}
		if !allowedTemplateFields[field] {
			return fmt.Errorf("%w: template field %q", ErrUnsupported, field)
		}
	}
	return nil
}

// maintenanceRangeRegex matches a maintenance branch name such as
// "1.x" or "1.2.x", as semantic-release's isMaintenanceRange does.
var maintenanceRangeRegex = regexp.MustCompile(`(?i)^\d+(?:\.(?:\d+|x))?\.x$`)

// parseBranches validates the branches list as semantic-release does.
// The release command supports one release branch and any number of
// prerelease branches; maintenance branches and more release branches
// need range logic that it does not have. The release branch gets the
// default channel and every prerelease branch is its own channel.
func parseBranches(nodes []yaml.Node) ([]Branch, error) {
	out := make([]Branch, 0, len(nodes))
	names, prereleases := map[string]bool{}, map[string]bool{}
	releaseBranches := 0
	for i := range nodes {
		b, err := parseBranch(&nodes[i])
		if err != nil {
			return nil, err
		}
		if names[b.Name] {
			return nil, fmt.Errorf("%w: duplicate branch %q", ErrUnsupported, b.Name)
		}
		names[b.Name] = true
		if b.IsPrerelease() {
			if prereleases[b.Prerelease] {
				return nil, fmt.Errorf("%w: two branches with prerelease %q", ErrUnsupported, b.Prerelease)
			}
			prereleases[b.Prerelease] = true
		} else {
			releaseBranches++
		}
		out = append(out, b)
	}
	if releaseBranches != 1 {
		return nil, fmt.Errorf("%w: %d release branches (exactly one is supported)", ErrUnsupported, releaseBranches)
	}
	return out, nil
}

// parseBranch validates one branches entry, which is a name or an object.
func parseBranch(n *yaml.Node) (Branch, error) {
	var rb rawBranch
	if n.Kind == yaml.ScalarNode {
		rb.Name = n.Value
	} else if err := decodeNode(n, &rb, "branches"); err != nil {
		return Branch{}, err
	}
	if !validBranchName(rb.Name) {
		return Branch{}, fmt.Errorf("%w: invalid branch name %q", ErrUnsupported, rb.Name)
	}
	if maintenanceRangeRegex.MatchString(rb.Name) {
		return Branch{}, fmt.Errorf("%w: maintenance branch %q", ErrUnsupported, rb.Name)
	}
	b := Branch{Name: rb.Name}
	p := &rb.Prerelease
	switch {
	case p.Kind == 0 || p.Tag == "!!null" || (p.Tag == "!!bool" && p.Value == "false"):
		return b, nil
	case p.Tag == "!!bool" && p.Value == "true":
		b.Prerelease = rb.Name
	case p.Tag == "!!str":
		b.Prerelease = p.Value
	default:
		return Branch{}, fmt.Errorf("%w: branch %q prerelease %q", ErrUnsupported, rb.Name, p.Value)
	}
	// As semantic-release does, check that 1.0.0-<id>.1 is valid. This
	// also rejects an empty identifier.
	if _, err := semver.StrictNewVersion("1.0.0-" + b.Prerelease + ".1"); err != nil {
		return Branch{}, fmt.Errorf("%w: branch %q prerelease identifier %q", ErrUnsupported, rb.Name, b.Prerelease)
	}
	// semantic-release compares the id with each part of a version's
	// prerelease, and node-semver stores a numeric part as a number, so
	// it never finds the last release of a branch with a numeric or
	// dotted id.
	if strings.Contains(b.Prerelease, ".") || strings.Trim(b.Prerelease, "0123456789") == "" {
		return Branch{}, fmt.Errorf("%w: branch %q prerelease identifier %q never matches its tags",
			ErrUnsupported, rb.Name, b.Prerelease)
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
func parseRule(n *yaml.Node) (Rule, error) {
	var r Rule
	if n.Kind != yaml.MappingNode {
		return r, fmt.Errorf("%w: release rule at line %d is not an object", ErrUnsupported, n.Line)
	}
	if err := decodeNode(n, &r, "releaseRules"); err != nil {
		return r, err
	}
	if !releaseValues[r.Release] {
		return r, fmt.Errorf("%w: release rule with release %q", ErrUnsupported, r.Release)
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		key, value := n.Content[i].Value, n.Content[i+1].Value
		if key == "type" || key == "scope" || key == "subject" {
			if err := validateGlob(value); err != nil {
				return r, fmt.Errorf("%w: release rule %s %q: %w", ErrUnsupported, key, value, err)
			}
		}
	}
	return r, nil
}

// errEmptyGlob is returned for an empty rule glob, which makes micromatch
// throw.
var errEmptyGlob = errors.New("empty pattern")

// errUnsupportedGlob is returned for glob syntax outside the allowlist of
// validateGlob.
var errUnsupportedGlob = errors.New("unsupported glob syntax")

// validateGlob checks that a rule glob uses only syntax that path.Match
// and micromatch read the same way: literal characters, "*", "?", a
// "[...]" or "[^...]" class of letters, digits, ranges, "_" and ".", and
// "\" before punctuation. Everything else fails closed, because
// micromatch has more syntax (negation, braces, extglobs, globstars,
// POSIX classes) that path.Match reads in another way.
func validateGlob(p string) error {
	if p == "" {
		return errEmptyGlob
	}
	if strings.HasPrefix(p, "./") {
		return errUnsupportedGlob // micromatch removes a leading "./".
	}
	for i := 0; i < len(p); i++ {
		c := p[i]
		switch {
		case c == '*' && strings.HasPrefix(p[i+1:], "*"):
			return errUnsupportedGlob // a globstar
		case c == '*' || c == '?':
		case c == '\\':
			i++
			if i == len(p) || isGlobAlnum(p[i]) || p[i] < '!' || p[i] > '~' {
				return errUnsupportedGlob
			}
		case c == '[':
			end := globClassEnd(p, i)
			if end < 0 {
				return errUnsupportedGlob
			}
			i = end
		case isGlobAlnum(c) || c >= 0x80 || strings.IndexByte(" -_.,:;/#'%&=~$^<>+@", c) >= 0:
		default:
			return errUnsupportedGlob
		}
	}
	return nil
}

// MatchGlob reports whether value matches a rule glob as micromatch.isMatch
// does, for the patterns that validateGlob accepts. An empty pattern means
// that the rule has no such key, and it matches anything. micromatch
// never matches an empty or missing value, and a path segment pattern
// that starts with "*" or "?" does not match a segment that starts with
// a dot.
func MatchGlob(pattern, value string) bool {
	if pattern == "" {
		return true
	}
	if value == "" {
		return false
	}
	if ok, err := path.Match(pattern, value); err != nil || !ok {
		return false
	}
	// path.Match matched, so both have the same number of segments.
	for value != "" {
		var seg, p string
		seg, value, _ = strings.Cut(value, "/")
		p, pattern, _ = strings.Cut(pattern, "/")
		if strings.HasPrefix(seg, ".") && (strings.HasPrefix(p, "*") || strings.HasPrefix(p, "?")) {
			return false
		}
	}
	return true
}

// globClassEnd returns the index of the "]" that closes the class that
// starts at p[start], or -1 if the class is not in the allowlist of
// validateGlob.
func globClassEnd(p string, start int) int {
	i := start + 1
	if i < len(p) && p[i] == '^' {
		i++
	}
	first := i
	for ; i < len(p) && p[i] != ']'; i++ {
		c := p[i]
		switch {
		case isGlobAlnum(c) && i+2 < len(p) && p[i+1] == '-' && p[i+2] != ']':
			if !isGlobAlnum(p[i+2]) || p[i+2] < c {
				return -1
			}
			i += 2
		case isGlobAlnum(c) || c == '_' || c == '.':
		default:
			return -1
		}
	}
	if i == len(p) || i == first {
		return -1
	}
	return i
}

// isGlobAlnum reports whether c is an ASCII letter or digit.
func isGlobAlnum(c byte) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
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
	return "", nil, fmt.Errorf("%w: plugin entry at line %d", ErrUnsupported, n.Line)
}

// decodeNode decodes a YAML node into v, rejects unknown keys, and
// wraps a failure in ErrUnsupported.
func decodeNode(n *yaml.Node, v any, what string) error {
	if err := strictyaml.DecodeNode(n, v); err != nil {
		return fmt.Errorf("%w: %s: %w", ErrUnsupported, what, err)
	}
	return nil
}
