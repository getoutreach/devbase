// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Parses conventional commits and decides the release
// type, as @semantic-release/commit-analyzer does with the
// conventionalcommits preset.

package release

import (
	"path"
	"regexp"
	"strings"
)

// These patterns come from conventional-changelog-conventionalcommits.
var (
	// headerRegex matches a commit header.
	headerRegex = regexp.MustCompile(`^(\w*)(?:\((.*)\))?(!?): (.*)$`)

	// noteRegex matches a breaking change note line.
	noteRegex = regexp.MustCompile(`(?i)^[\s|*]*(BREAKING CHANGE|BREAKING-CHANGE)[:\s]+(.*)`)

	// revertRegex matches a revert commit and captures the reverted
	// header and hash.
	revertRegex = regexp.MustCompile(`(?is)^(?:Revert|revert:)\s"?([\s\S]+?)"?\s*This reverts commit (\w*)\.`)

	// skipReleaseRegex matches a commit that opts out of the release analysis.
	skipReleaseRegex = regexp.MustCompile(`(?i)\[skip\s+release]|\[release\s+skip]`)

	// closesRegex matches an issue reference such as "Closes #7".
	closesRegex = regexp.MustCompile(`(?i)\b(?:close[sd]?|fix(?:e[sd])?|resolve[sd]?)\s+#(\d+)`)
)

// defaultRules are the commit-analyzer default rules that apply to the
// conventionalcommits preset.
//
//nolint:gochecknoglobals // Why: read-only list.
var defaultRules = []Rule{
	{Breaking: true, Release: string(BumpMajor)},
	{Revert: true, Release: string(BumpPatch)},
	{Type: "feat", Release: string(BumpMinor)},
	{Type: "fix", Release: string(BumpPatch)},
	{Type: "perf", Release: string(BumpPatch)},
}

// Commit is a parsed conventional commit.
type Commit struct {
	Hash    string
	Message string
	Header  string
	Type    string

	// Scope is empty for no scope and for the "*" scope.
	Scope string

	// Subject is the header itself when the header has no type, such as
	// a git-style `Revert "..."` commit.
	Subject string

	// Notes are the breaking change notes.
	Notes []string

	// Revert is set when the commit reverts another commit.
	Revert *Revert

	// Closes are the issue numbers that the commit closes.
	Closes []string
}

// Revert identifies the commit that a revert commit reverts.
type Revert struct {
	Header string
	Hash   string
}

// ParseCommit parses a commit message.
func ParseCommit(raw RawCommit) Commit {
	c := Commit{Hash: raw.Hash, Message: raw.Message}
	lines := strings.Split(raw.Message, "\n")
	c.Header = strings.TrimSpace(lines[0])
	c.Subject = c.Header
	breakingHeader := false
	if m := headerRegex.FindStringSubmatch(c.Header); m != nil {
		c.Type, c.Scope, c.Subject = m[1], m[2], m[4]
		breakingHeader = m[3] == "!"
	}
	if c.Scope == "*" {
		c.Scope = ""
	}

	for i, line := range lines[1:] {
		m := noteRegex.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		// The note text runs to the next blank line.
		text := []string{strings.TrimSpace(m[2])}
		for _, next := range lines[i+2:] {
			if strings.TrimSpace(next) == "" {
				break
			}
			text = append(text, strings.TrimSpace(next))
		}
		c.Notes = append(c.Notes, strings.Join(text, "\n"))
	}
	if len(c.Notes) == 0 && breakingHeader {
		c.Notes = []string{c.Subject}
	}

	if m := revertRegex.FindStringSubmatch(raw.Message); m != nil {
		c.Revert = &Revert{Header: m[1], Hash: m[2]}
	}
	for _, m := range closesRegex.FindAllStringSubmatch(raw.Message, -1) {
		c.Closes = append(c.Closes, m[1])
	}
	return c
}

// ParseCommits parses raw commits and removes each pair of a commit and
// its revert, as conventional-changelog's revert filter does.
func ParseCommits(raws []RawCommit) []Commit {
	commits := make([]Commit, 0, len(raws))
	byHeader := map[string][]int{}
	for _, r := range raws {
		if strings.TrimSpace(r.Message) == "" {
			continue
		}
		c := ParseCommit(r)
		byHeader[c.Header] = append(byHeader[c.Header], len(commits))
		commits = append(commits, c)
	}

	drop := map[int]bool{}
	for i := range commits {
		rv := commits[i].Revert
		if rv == nil || drop[i] {
			continue
		}
		for _, j := range byHeader[rv.Header] {
			if j != i && !drop[j] && strings.HasPrefix(commits[j].Hash, rv.Hash) {
				drop[i], drop[j] = true, true
				break
			}
		}
	}
	out := commits[:0]
	for i := range commits {
		if !drop[i] {
			out = append(out, commits[i])
		}
	}
	return out
}

// Analyze returns the release type for a list of commits. Custom rules
// are checked first; a commit that matches no custom rule uses the
// default rules. Commits marked [skip release] are left out.
func Analyze(commits []Commit, rules []Rule) Bump {
	result := BumpNone
	for i := range commits {
		c := &commits[i]
		if skipReleaseRegex.MatchString(c.Message) {
			continue
		}
		t, matched := matchRules(c, rules)
		if !matched {
			t, _ = matchRules(c, defaultRules)
		}
		result = maxBump(result, t)
		if result == BumpMajor {
			break
		}
	}
	return result
}

// matchRules returns the highest release type of the rules that match
// c, and whether any rule matched. A matching rule with release "false"
// counts as a match with no release.
func matchRules(c *Commit, rules []Rule) (Bump, bool) {
	result, matched := BumpNone, false
	for _, r := range rules {
		if (r.Breaking && len(c.Notes) == 0) || (r.Revert && c.Revert == nil) {
			continue
		}
		if !globMatch(r.Type, c.Type) || !globMatch(r.Scope, c.Scope) || !globMatch(r.Subject, c.Subject) {
			continue
		}
		matched = true
		if r.Release != "false" {
			result = maxBump(result, Bump(r.Release))
		}
	}
	return result, matched
}

// globMatch reports whether value matches pattern. An empty pattern
// matches anything.
func globMatch(pattern, value string) bool {
	if pattern == "" {
		return true
	}
	ok, err := path.Match(pattern, value)
	return err == nil && ok
}
