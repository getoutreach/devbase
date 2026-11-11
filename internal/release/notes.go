// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Renders release notes in the format of the
// conventional-changelog-conventionalcommits preset.

package release

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"
)

// issueRefRegex matches an issue reference such as #123 in a subject.
var issueRefRegex = regexp.MustCompile(`#(\d+)`)

// NoteLinks builds the links in release notes. A forge implementation
// provides them, so that the renderer does not depend on one forge.
type NoteLinks interface {
	Compare(from, to string) string
	Commit(hash string) string
	Issue(number string) string
}

// commitGroup is one section of the notes, in preset order.
type commitGroup struct {
	Types  []string
	Title  string
	Hidden bool
}

// commitGroups are the conventionalcommits preset types, in order.
//
//nolint:gochecknoglobals // Why: read-only list.
var commitGroups = []commitGroup{
	{Types: []string{"feat", "feature"}, Title: "Features"},
	{Types: []string{"fix"}, Title: "Bug Fixes"},
	{Types: []string{"perf"}, Title: "Performance Improvements"},
	{Types: []string{"revert"}, Title: "Reverts"},
	{Types: []string{"docs"}, Title: "Documentation", Hidden: true},
	{Types: []string{"style"}, Title: "Styles", Hidden: true},
	{Types: []string{"chore"}, Title: "Miscellaneous Chores", Hidden: true},
	{Types: []string{"refactor"}, Title: "Code Refactoring", Hidden: true},
	{Types: []string{"test"}, Title: "Tests", Hidden: true},
	{Types: []string{"build"}, Title: "Build System", Hidden: true},
	{Types: []string{"ci"}, Title: "Continuous Integration", Hidden: true},
}

// NotesInput is the data that RenderNotes needs.
type NotesInput struct {
	// Version is the new version, without a "v" prefix.
	Version string

	// PreviousTag is the tag of the last release, or empty.
	PreviousTag string

	Date    time.Time
	Commits []Commit
	Links   NoteLinks
}

// RenderNotes renders release notes for the commits of a release.
func RenderNotes(in *NotesInput) string {
	date := in.Date.UTC().Format(time.DateOnly)
	header := fmt.Sprintf("## %s (%s)", in.Version, date)
	if in.PreviousTag != "" {
		header = fmt.Sprintf("## [%s](%s) (%s)", in.Version, in.Links.Compare(in.PreviousTag, "v"+in.Version), date)
	}
	sections := []string{header}

	// Notes keep commit order, as conventional-changelog does.
	var notes []string
	for i := range in.Commits {
		for _, n := range in.Commits[i].Notes {
			notes = append(notes, "* "+scopePrefix(in.Commits[i].Scope)+n)
		}
	}
	if len(notes) > 0 {
		sections = append(sections, "### ⚠ BREAKING CHANGES\n\n"+strings.Join(notes, "\n"))
	}

	for _, g := range commitGroups {
		var lines []commitLine
		for i := range in.Commits {
			c := &in.Commits[i]
			if slices.Contains(g.Types, noteType(c)) && (!g.Hidden || len(c.Notes) > 0) {
				lines = append(lines, renderCommit(c, in.Links))
			}
		}
		if len(lines) == 0 {
			continue
		}
		slices.SortStableFunc(lines, func(a, b commitLine) int { return strings.Compare(a.sortKey, b.sortKey) })
		texts := make([]string, len(lines))
		for i, l := range lines {
			texts[i] = l.text
		}
		sections = append(sections, "### "+g.Title+"\n\n"+strings.Join(texts, "\n"))
	}
	return strings.Join(sections, "\n\n") + "\n"
}

// commitLine is one rendered commit with its sort key.
type commitLine struct {
	sortKey string
	text    string
}

// noteType returns the type that decides the section of a commit. A
// revert goes under Reverts whatever its header type is.
func noteType(c *Commit) string {
	if c.Revert != nil {
		return "revert"
	}
	return c.Type
}

// renderCommit renders one commit line.
func renderCommit(c *Commit, links NoteLinks) commitLine {
	inSubject := map[string]bool{}
	subject := issueRefRegex.ReplaceAllStringFunc(c.Subject, func(m string) string {
		inSubject[m[1:]] = true
		return fmt.Sprintf("[%s](%s)", m, links.Issue(m[1:]))
	})
	short := c.Hash[:min(7, len(c.Hash))]
	line := fmt.Sprintf("* %s%s ([%s](%s))", scopePrefix(c.Scope), subject, short, links.Commit(c.Hash))
	var closes []string
	for _, n := range c.Closes {
		if !inSubject[n] {
			closes = append(closes, fmt.Sprintf("[#%s](%s)", n, links.Issue(n)))
		}
	}
	if len(closes) > 0 {
		line += ", closes " + strings.Join(closes, " ")
	}
	return commitLine{sortKey: c.Scope + c.Subject, text: line}
}

// scopePrefix returns the bold scope prefix of a line, or nothing.
func scopePrefix(scope string) string {
	if scope == "" {
		return ""
	}
	return "**" + scope + ":** "
}
