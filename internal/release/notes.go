// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Renders release notes as the writer of the
// conventional-changelog-conventionalcommits preset does.

package release

import (
	"cmp"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"

	"golang.org/x/text/collate"
	"golang.org/x/text/language"
)

// These patterns come from conventional-changelog-conventionalcommits.
var (
	// releaseAsRegex matches a Release-As footer, which keeps a commit
	// in the notes even when its type is hidden.
	releaseAsRegex = regexp.MustCompile(foldASCII("release-as:") + jsSpace +
		`*\w*@?([0-9]+\.[0-9]+\.[0-9a-zA-Z]+(-[0-9a-zA-Z.]+)?)` + jsSpace + `*`)

	// subjectIssueRegex matches an issue in a subject. It is case
	// sensitive, as in the preset.
	subjectIssueRegex = regexp.MustCompile(`(#)([a-z0-9]+)`)

	// userRegex matches a code span, which is left as is, or a user
	// mention.
	userRegex = regexp.MustCompile("`[^`]*`|\\B@([a-z0-9](?:-?[a-z0-9/]){0,38})")
)

// NoteLinks builds the links in release notes. A forge implementation
// provides them, so that the renderer does not depend on one forge.
type NoteLinks interface {
	// Compare returns the URL that compares two tags.
	Compare(from, to string) string

	// Commit returns the URL of a commit.
	Commit(hash string) string

	// Issue returns the URL of an issue. An empty owner or repo means
	// the owner or repository of the release.
	Issue(owner, repo, id string) string

	// User returns the URL of a user.
	User(name string) string
}

// commitType is a type entry of the conventionalcommits preset.
type commitType struct {
	Type    string
	Section string
	Hidden  bool
}

// commitTypes are the conventionalcommits preset types. Their order
// sets the order of the sections.
//
//nolint:gochecknoglobals // Why: read-only list.
var commitTypes = []commitType{
	{Type: "feat", Section: "Features"},
	{Type: "feature", Section: "Features"},
	{Type: "fix", Section: "Bug Fixes"},
	{Type: "perf", Section: "Performance Improvements"},
	{Type: "revert", Section: "Reverts"},
	{Type: "docs", Section: "Documentation", Hidden: true},
	{Type: "style", Section: "Styles", Hidden: true},
	{Type: "chore", Section: "Miscellaneous Chores", Hidden: true},
	{Type: "refactor", Section: "Code Refactoring", Hidden: true},
	{Type: "test", Section: "Tests", Hidden: true},
	{Type: "build", Section: "Build System", Hidden: true},
	{Type: "ci", Section: "Continuous Integration", Hidden: true},
}

// NotesInput is the data that RenderNotes needs.
type NotesInput struct {
	// Version is the new version, without a "v" prefix.
	Version string

	// Tag is the tag of the new version.
	Tag string

	// PreviousTag is the tag of the last release, or empty.
	PreviousTag string

	Date    time.Time
	Commits []Commit
	Links   NoteLinks
}

// noteCommit is a commit after the preset's transform step.
type noteCommit struct {
	// Group is the section title, the raw type for an unknown type, or
	// empty for a commit with no type.
	Group string

	Scope string

	// Text is the subject with links, or the header when there is no
	// subject.
	Text string

	// SortKey is the scope and the subject with links.
	SortKey string

	Hash  string
	Notes []Note
	Refs  []Reference
}

// RenderNotes renders release notes for the commits from ParseCommits.
// Reverted commits are left out.
func RenderNotes(in *NotesInput) string {
	var commits []noteCommit
	for _, c := range filterReverted(in.Commits) {
		if nc, ok := transformCommit(&c, in.Links); ok {
			commits = append(commits, nc)
		}
	}

	var b strings.Builder
	date := in.Date.UTC().Format(time.DateOnly)
	if in.PreviousTag != "" && in.Tag != "" {
		b.WriteString("## [" + in.Version + "](" + in.Links.Compare(in.PreviousTag, in.Tag) + ") (" + date + ")\n")
	} else {
		b.WriteString("## " + in.Version + " (" + date + ")\n")
	}

	// Notes keep commit order: the preset's notesSort treats all notes
	// as equal.
	breaking := false
	for i := range commits {
		for _, n := range commits[i].Notes {
			if !breaking {
				b.WriteString("\n### ⚠ BREAKING CHANGES\n\n")
				breaking = true
			}
			b.WriteString("* " + scopePrefix(commits[i].Scope) + n.Text + "\n")
		}
	}

	for _, g := range groupCommits(commits) {
		b.WriteString("\n")
		if g.title != "" {
			b.WriteString("### " + g.title + "\n\n")
		}
		for i := range g.commits {
			b.WriteString(renderCommit(&g.commits[i], in.Links) + "\n")
		}
	}
	return jsTrim(b.String()) + "\n"
}

// transformCommit applies the preset's transform step. It returns false
// for a commit that the notes leave out: a commit with a hidden or
// unknown type, unless it has a breaking change note or a Release-As
// footer.
func transformCommit(c *Commit, links NoteLinks) (noteCommit, bool) {
	entry := findCommitType(c)
	if (entry == nil || entry.Hidden) && len(c.Notes) == 0 &&
		!releaseAsRegex.MatchString(c.Footer) && !releaseAsRegex.MatchString(c.Body) {
		return noteCommit{}, false
	}

	nc := noteCommit{Group: c.Type, Scope: c.Scope, Hash: c.Hash, Notes: c.Notes}
	if entry != nil {
		nc.Group = entry.Section
	}
	if nc.Scope == "*" {
		nc.Scope = ""
	}

	subject, issues := linkSubject(c.Subject, links)
	nc.SortKey = nc.Scope + subject
	nc.Text = subject
	if nc.Text == "" {
		nc.Text = c.Header
	}
	for _, r := range c.References {
		if !slices.Contains(issues, r.Prefix+r.Issue) {
			nc.Refs = append(nc.Refs, r)
		}
	}
	return nc, true
}

// findCommitType returns the preset type entry of a commit, or nil. A
// revert commit uses the "revert" entry whatever its type is.
func findCommitType(c *Commit) *commitType {
	key := strings.ToLower(c.Type)
	if c.Revert != nil {
		key = "revert"
	}
	if i := slices.IndexFunc(commitTypes, func(t commitType) bool { return t.Type == key }); i >= 0 {
		return &commitTypes[i]
	}
	return nil
}

// linkSubject adds issue and user links to a subject. It returns the
// new subject and the issues it links, such as "#12".
func linkSubject(subject string, links NoteLinks) (linked string, issues []string) {
	subject = subjectIssueRegex.ReplaceAllStringFunc(subject, func(m string) string {
		issues = append(issues, m)
		return "[" + m + "](" + links.Issue("", "", m[1:]) + ")"
	})

	subject = userRegex.ReplaceAllStringFunc(subject, func(m string) string {
		// A code span is left as is. A user with a "/" is not linked.
		if m[0] == '`' || strings.Contains(m, "/") {
			return m
		}
		return "[" + m + "](" + links.User(m[1:]) + ")"
	})
	return subject, issues
}

// commitGroup is a section of the notes.
type commitGroup struct {
	title   string
	commits []noteCommit
}

// groupCommits groups commits by section, as the writer's getCommitGroups
// does. The groups start in JavaScript object key order (keys that are
// array indexes first, in numeric order, then the rest by first use),
// then are sorted by the preset section order, with unknown sections
// first. Commits in a group are sorted by scope and subject with the
// en-US collation, which is what localeCompare uses in Node.js.
func groupCommits(commits []noteCommit) []commitGroup {
	var groups []commitGroup
	for i := range commits {
		k := slices.IndexFunc(groups, func(g commitGroup) bool { return g.title == commits[i].Group })
		if k < 0 {
			groups = append(groups, commitGroup{title: commits[i].Group})
			k = len(groups) - 1
		}
		groups[k].commits = append(groups[k].commits, commits[i])
	}
	slices.SortStableFunc(groups, func(a, b commitGroup) int {
		ia, aIndex := arrayIndex(a.title)
		ib, bIndex := arrayIndex(b.title)
		switch {
		case aIndex && bIndex:
			return cmp.Compare(ia, ib)
		case aIndex:
			return -1
		case bIndex:
			return 1
		}
		return 0
	})
	slices.SortStableFunc(groups, func(a, b commitGroup) int {
		return cmp.Compare(sectionRank(a.title), sectionRank(b.title))
	})
	coll := collate.New(language.AmericanEnglish)
	for _, g := range groups {
		slices.SortStableFunc(g.commits, func(a, b noteCommit) int { return coll.CompareString(a.SortKey, b.SortKey) })
	}
	return groups
}

// arrayIndex reports whether s is a JavaScript array index, which an
// object lists before its other keys, and returns its value.
func arrayIndex(s string) (uint64, bool) {
	if s == "" || (len(s) > 1 && s[0] == '0') {
		return 0, false
	}
	n, err := strconv.ParseUint(s, 10, 32)
	return n, err == nil && n < 1<<32-1 && strconv.FormatUint(n, 10) == s
}

// sectionRank returns the position of a section title in the preset
// order, or -1 for an unknown title.
func sectionRank(title string) int {
	return slices.IndexFunc(commitTypes, func(t commitType) bool { return t.Section == title })
}

// renderCommit renders one commit line.
func renderCommit(c *noteCommit, links NoteLinks) string {
	line := "* " + scopePrefix(c.Scope) + c.Text +
		" ([" + shortHash(c.Hash) + "](" + links.Commit(c.Hash) + "))"
	if len(c.Refs) == 0 {
		return line
	}
	var b strings.Builder
	b.WriteString(line + ", closes")
	for _, r := range c.Refs {
		name := r.Repository + r.Prefix + r.Issue
		if r.Owner != "" {
			name = r.Owner + "/" + name
		}
		b.WriteString(" [" + name + "](" + links.Issue(r.Owner, r.Repository, r.Issue) + ")")
	}
	return b.String()
}

// shortHash returns the first 7 UTF-16 code units of a hash, as
// JavaScript's substring does. A "-hash-" meta line can make the hash
// any text.
func shortHash(hash string) string {
	units := utf16.Encode([]rune(hash))
	return string(utf16.Decode(units[:min(7, len(units))]))
}

// scopePrefix returns the bold scope prefix of a line, or nothing.
func scopePrefix(scope string) string {
	if scope == "" {
		return ""
	}
	return "**" + scope + ":** "
}
