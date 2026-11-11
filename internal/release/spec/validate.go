// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: The rules that every release config must keep, whichever
// loader made it, so that the release command fails closed.

package spec

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/Masterminds/semver/v3"
)

// maintenanceRangeRegex matches a maintenance branch name such as "1.x"
// or "1.2.x", as semantic-release's isMaintenanceRange does.
var maintenanceRangeRegex = regexp.MustCompile(`(?i)^\d+(?:\.(?:\d+|x))?\.x$`)

// envItemRegex matches a NAME=value build environment item.
var envItemRegex = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*=`)

// releaseValues are the valid Release values of a rule.
//
//nolint:gochecknoglobals // Why: read-only lookup table.
var releaseValues = map[Bump]bool{BumpPatch: true, BumpMinor: true, BumpMajor: true, BumpNoRelease: true}

// Validate checks the rules of the model that the release command needs
// to reproduce semantic-release and GoReleaser, and returns warnings for
// problems that do not stop a release. Every error wraps ErrUnsupported.
func (c *Config) Validate() (warnings []string, err error) {
	if err := validateBranches(c.Branches); err != nil {
		return nil, err
	}
	for i := range c.Rules {
		if err := c.Rules[i].validate(); err != nil {
			return nil, err
		}
	}
	for i := range c.Plugins {
		w, err := c.Plugins[i].validate()
		if err != nil {
			return nil, err
		}
		warnings = append(warnings, w...)
	}
	if c.Artifacts != nil {
		if err := c.Artifacts.Validate(); err != nil {
			return nil, err
		}
	}
	return warnings, nil
}

// validateBranches checks the branches as semantic-release does. The
// release command supports one release branch and any number of
// prerelease branches; maintenance branches and more release branches
// need range logic that it does not have.
func validateBranches(branches []Branch) error {
	names, prereleases := map[string]bool{}, map[string]bool{}
	releaseBranches := 0
	for i := range branches {
		b := &branches[i]
		if err := b.validate(); err != nil {
			return err
		}
		if names[b.Name] {
			return fmt.Errorf("%w: duplicate branch %q", ErrUnsupported, b.Name)
		}
		names[b.Name] = true
		if b.IsPrerelease() {
			if prereleases[b.Prerelease] {
				return fmt.Errorf("%w: two branches with prerelease %q", ErrUnsupported, b.Prerelease)
			}
			prereleases[b.Prerelease] = true
		} else {
			releaseBranches++
		}
	}
	if releaseBranches != 1 {
		return fmt.Errorf("%w: %d release branches (exactly one is supported)", ErrUnsupported, releaseBranches)
	}
	return nil
}

// validate checks one branch. The release branch has the default
// channel; a prerelease branch has a channel.
func (b *Branch) validate() error {
	switch {
	case !validBranchName(b.Name):
		return fmt.Errorf("%w: invalid branch name %q", ErrUnsupported, b.Name)
	case maintenanceRangeRegex.MatchString(b.Name):
		return fmt.Errorf("%w: maintenance branch %q", ErrUnsupported, b.Name)
	case !b.IsPrerelease():
		if b.Channel != "" {
			return fmt.Errorf("%w: release branch %q with channel %q", ErrUnsupported, b.Name, b.Channel)
		}
		return nil
	case b.Channel == "":
		return fmt.Errorf("%w: prerelease branch %q without a channel", ErrUnsupported, b.Name)
	}
	// As semantic-release does, check that 1.0.0-<id>.1 is valid.
	if _, err := semver.StrictNewVersion("1.0.0-" + b.Prerelease + ".1"); err != nil {
		return fmt.Errorf("%w: branch %q prerelease identifier %q", ErrUnsupported, b.Name, b.Prerelease)
	}
	// semantic-release compares the id with each part of a version's
	// prerelease, and node-semver stores a numeric part as a number, so
	// it never finds the last release of a branch with a numeric or
	// dotted id.
	if strings.Contains(b.Prerelease, ".") || strings.Trim(b.Prerelease, "0123456789") == "" {
		return fmt.Errorf("%w: branch %q prerelease identifier %q never matches its tags",
			ErrUnsupported, b.Name, b.Prerelease)
	}
	return nil
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

// validate checks a rule's release value and its globs.
func (r *Rule) validate() error {
	if !releaseValues[r.Release] {
		return fmt.Errorf("%w: release rule with release %q", ErrUnsupported, r.Release)
	}
	for _, f := range []struct{ name, glob string }{{"type", r.Type}, {"scope", r.Scope}, {"subject", r.Subject}} {
		if f.glob == "" {
			continue // No such field in the rule.
		}
		if err := ValidateGlob(f.glob); err != nil {
			return fmt.Errorf("release rule %s %q: %w", f.name, f.glob, err)
		}
	}
	return nil
}

// validateFileGlobs checks the file globs of a forge release or a release
// commit. The publish step matches them with filepath.Glob, so the globby
// and micromatch syntax that it reads in another way is rejected: "**",
// braces, and a leading "!".
func validateFileGlobs(globs []string) error {
	for _, g := range globs {
		if g == "" || strings.Contains(g, "**") || strings.ContainsAny(g, "{}") || strings.HasPrefix(g, "!") {
			return fmt.Errorf("%w: file glob %q", ErrUnsupported, g)
		}
	}
	return nil
}

// validate checks that exactly one capability is set, with valid options.
func (p *Plugin) validate() (warnings []string, err error) {
	set := 0
	for _, isSet := range []bool{p.Notes != nil, p.Forge != nil, p.Commit != nil, p.NPM != nil, p.Orb != nil, p.Exec != nil} {
		if isSet {
			set++
		}
	}
	switch {
	case set != 1:
		return nil, fmt.Errorf("%w: a plugin with %d capabilities", ErrUnsupported, set)
	case p.Forge != nil && p.Forge.Provider != ForgeGitHub:
		return nil, fmt.Errorf("%w: forge %q", ErrUnsupported, p.Forge.Provider)
	case p.Orb != nil && p.Orb.Name == "":
		return nil, fmt.Errorf("%w: orb plugin without a name", ErrUnsupported)
	case p.Forge != nil:
		return nil, validateFileGlobs(p.Forge.Assets)
	case p.Commit != nil:
		return nil, validateFileGlobs(p.Commit.Files)
	case p.Exec != nil:
		for _, cmd := range []string{p.Exec.GenerateNotes, p.Exec.Prepare, p.Exec.Publish, p.Exec.Success} {
			w, err := CheckExecCommand(cmd)
			if err != nil {
				return nil, err
			}
			warnings = append(warnings, w...)
		}
	}
	return warnings, nil
}

// Validate checks the artifacts: every build has an id, a binary name
// and ldflags; ids, binary names and targets are unique; targets have
// their default variant; every template uses only the supported fields;
// and the archive sources stay in the repository.
func (a *Artifacts) Validate() error {
	switch {
	case len(a.Builds) == 0:
		return fmt.Errorf("%w: no builds", ErrUnsupported)
	case a.Checksums == "" || strings.ContainsAny(a.Checksums, `/\`) || strings.Contains(a.Checksums, "{{"):
		return fmt.Errorf("%w: checksum file name %q", ErrUnsupported, a.Checksums)
	}
	for _, hook := range a.Hooks {
		if err := CheckTemplate(hook); err != nil {
			return fmt.Errorf("before hook %q: %w", hook, err)
		}
	}
	targets := map[string]bool{}
	for _, t := range a.Targets {
		switch {
		case t != NewTarget(t.OS, t.Arch):
			return fmt.Errorf("%w: target %s without its default variant", ErrUnsupported, &t)
		case targets[t.String()]:
			return fmt.Errorf("%w: duplicate target %s", ErrUnsupported, &t)
		}
		targets[t.String()] = true
	}
	ids, binaries := map[string]bool{}, map[string]bool{}
	for i := range a.Builds {
		b := &a.Builds[i]
		if err := b.validate(); err != nil {
			return fmt.Errorf("build %q: %w", b.ID, err)
		}
		if ids[b.ID] {
			return fmt.Errorf("%w: two builds with id %q", ErrUnsupported, b.ID)
		}
		ids[b.ID] = true
		// Every binary goes in one archive.
		if binaries[b.Binary] {
			return fmt.Errorf("%w: two builds with binary %q", ErrUnsupported, b.Binary)
		}
		binaries[b.Binary] = true
	}
	for _, f := range a.Files {
		if err := CheckTemplate(f.Src); err != nil {
			return fmt.Errorf("archive file %q: %w", f.Src, err)
		}
		// The builder checks a template src after it renders it.
		if !strings.Contains(f.Src, "{{") {
			if err := ValidateArchiveSrc(f.Src); err != nil {
				return err
			}
		}
	}
	return nil
}

// validate checks one build.
func (b *Build) validate() error {
	switch {
	case b.ID == "":
		return fmt.Errorf("%w: no id", ErrUnsupported)
	case b.Binary == "":
		return fmt.Errorf("%w: no binary", ErrUnsupported)
	case strings.Contains(b.Binary, "{{"):
		return fmt.Errorf("%w: binary is a template", ErrUnsupported)
	case b.Main == "":
		return fmt.Errorf("%w: no main package", ErrUnsupported)
	case len(b.LDFlags) == 0:
		return fmt.Errorf("%w: no ldflags", ErrUnsupported)
	}
	for _, f := range b.LDFlags {
		if err := CheckTemplate(f); err != nil {
			return fmt.Errorf("ldflags %q: %w", f, err)
		}
	}
	for _, e := range b.Env {
		if !envItemRegex.MatchString(e) {
			return fmt.Errorf("%w: env item %q is not NAME=value", ErrUnsupported, e)
		}
		if err := CheckTemplate(e); err != nil {
			return fmt.Errorf("env item %q: %w", e, err)
		}
	}
	return nil
}
