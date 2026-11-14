// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Matching paths against CODEOWNERS patterns with the rules of GitHub.

package codeowners

import (
	"path"
	"strings"
)

// segmentKind is the kind of a segment of a pattern.
type segmentKind int

// The kinds of segments.
const (
	// literal matches a path segment that is equal to the text of the segment.
	literal segmentKind = iota

	// stars matches a path segment that matches the text of the segment, which has "*" and not
	// "?".
	stars

	// glob matches a path segment that matches the text of the segment with path.Match.
	glob

	// anySegments is "**", which matches any number of path segments.
	anySegments
)

// segment is a part of a pattern between two "/".
type segment struct {
	// text is the text of the segment.
	text string

	// kind is the kind of the segment.
	kind segmentKind

	// parts are the parts of the text between the "*" of a stars segment.
	parts []string
}

// Pattern is a compiled CODEOWNERS pattern. GitHub uses the rules of gitignore files for
// CODEOWNERS patterns, with some differences. The rules are:
//
//   - "*" matches any sequence of characters except "/", and "?" matches any one character
//     except "/".
//   - A "**" segment matches any number of segments: "**/x" matches x in any directory, "x/**"
//     matches everything below directory x, and "a/**/b" matches "a/b", "a/x/b", "a/x/y/b", and
//     so on.
//   - A pattern with a "/" at the start or in the middle matches paths from the root of the
//     repository. Another pattern matches at any level.
//   - A pattern that ends with "/" only matches directories.
//   - A pattern that matches a directory also matches everything below the directory.
//   - When the last segment of a pattern that does not end with "/" has "*" or "?", the pattern
//     only matches whole paths: "docs/*" matches "docs/a.md", but not "docs/x/a.md". This rule
//     comes from an example in the GitHub documentation, and gitignore files do not have it.
//   - GitHub does not support negation with "!", character ranges with "[ ]", or escaping a
//     leading "#" with "\". These characters, and "\" in any position, only match themselves.
//   - Matching is case-sensitive.
//
// A pattern without segments, such as "/", matches nothing.
type Pattern struct {
	// segments match the segments of a path from the root. The segments of a pattern that is
	// not anchored at the root start with "**".
	segments []segment

	// dirOnly is true when the pattern ends with "/".
	dirOnly bool

	// wholePath is true when the pattern does not match the directories of a path.
	wholePath bool
}

// CompilePattern returns the compiled form of a CODEOWNERS pattern.
func CompilePattern(pattern string) Pattern {
	var p Pattern

	rest, dirOnly := strings.CutSuffix(pattern, "/")
	rest, anchored := strings.CutPrefix(rest, "/")
	if rest == "" {
		return p
	}

	p.dirOnly = dirOnly
	if !anchored && !strings.Contains(rest, "/") {
		p.segments = append(p.segments, segment{text: "**", kind: anySegments})
	}
	for text := range strings.SplitSeq(rest, "/") {
		seg := segment{text: text}
		switch {
		case text == "**":
			seg.kind = anySegments
		case strings.Contains(text, "?"):
			// For path.Match, "\" and "[" only match themselves when they are escaped.
			seg = segment{text: strings.NewReplacer(`\`, `\\`, "[", `\[`).Replace(text), kind: glob}
		case strings.Contains(text, "*"):
			seg = segment{text: text, kind: stars, parts: strings.Split(text, "*")}
		}
		p.segments = append(p.segments, seg)
	}

	last := p.segments[len(p.segments)-1].kind
	p.wholePath = !dirOnly && (last == stars || last == glob)

	return p
}

// Match reports whether the pattern matches rel, a path relative to the root of the repository
// with "/" as the separator. A path that ends with "/" is a directory, and another path is a file.
func (p Pattern) Match(rel string) bool {
	rel, isDir := strings.CutSuffix(rel, "/")
	if len(p.segments) == 0 || rel == "" {
		return false
	}

	// Most paths do not match, so check the last segment first when it must match.
	if p.wholePath && !p.segments[len(p.segments)-1].match(rel[strings.LastIndexByte(rel, '/')+1:]) {
		return false
	}

	return p.match(p.segments, rel, isDir)
}

// FirstSegment returns the first segment of every path that the pattern matches. It returns
// false when the pattern can match paths that start with different segments.
func (p Pattern) FirstSegment() (string, bool) {
	if len(p.segments) == 0 || p.segments[0].kind != literal {
		return "", false
	}

	return p.segments[0].text, true
}

// match reports whether segs match rel, the part of the path of Match that starts at a segment.
// isDir is true when the path of Match is a directory.
func (p Pattern) match(segs []segment, rel string, isDir bool) bool {
	seg := segs[0]
	if seg.kind == anySegments {
		if len(segs) == 1 {
			// "**" at the end matches one or more segments. When it does not match all of them,
			// the pattern matches a directory.
			return strings.Contains(rel, "/") || isDir || !p.dirOnly
		}

		// "**" in another place matches zero or more segments.
		for {
			if p.match(segs[1:], rel, isDir) {
				return true
			}

			i := strings.IndexByte(rel, '/')
			if i < 0 {
				return false
			}
			rel = rel[i+1:]
		}
	}

	name, rest, more := strings.Cut(rel, "/")
	if !seg.match(name) {
		return false
	}

	switch {
	case len(segs) > 1:
		return more && p.match(segs[1:], rest, isDir)
	case more:
		// The pattern matches a directory of the path.
		return !p.wholePath
	default:
		return isDir || !p.dirOnly
	}
}

// match reports whether the segment matches name, a segment of a path.
func (s segment) match(name string) bool {
	switch s.kind {
	case literal:
		return s.text == name
	case stars:
		return matchStars(s.parts, name)
	case glob:
		// The text of a glob segment is a valid pattern for path.Match, so err is always nil.
		ok, err := path.Match(s.text, name)

		return ok && err == nil
	case anySegments:
		return true
	default:
		return false
	}
}

// matchStars reports whether name matches a text whose only wildcard is "*". parts are the
// parts of the text between the "*". path.Match does the same, but it is slower.
func matchStars(parts []string, name string) bool {
	rest, ok := strings.CutPrefix(name, parts[0])
	if !ok {
		return false
	}

	// The first match of each middle part leaves the most room for the parts after it.
	for _, part := range parts[1 : len(parts)-1] {
		if _, rest, ok = strings.Cut(rest, part); !ok {
			return false
		}
	}

	return strings.HasSuffix(rest, parts[len(parts)-1])
}

// Matcher finds the entry of a CODEOWNERS file that applies to a path, which is the last entry
// whose pattern matches the path.
type Matcher struct {
	// entries are the entries of the CODEOWNERS file.
	entries []Entry

	// patterns are the compiled patterns of the entries.
	patterns []Pattern

	// byFirst maps the first segment of a path to the indexes of the entries whose patterns only
	// match paths that start with the segment, in increasing order.
	byFirst map[string][]int

	// anyFirst are the indexes of the other entries, in increasing order.
	anyFirst []int
}

// NewMatcher returns a Matcher for the entries of a CODEOWNERS file, in the order of the file.
func NewMatcher(entries []Entry) *Matcher {
	m := &Matcher{entries: entries, patterns: make([]Pattern, len(entries)), byFirst: make(map[string][]int)}
	for i, entry := range entries {
		m.patterns[i] = CompilePattern(entry.Pattern)
		if first, ok := m.patterns[i].FirstSegment(); ok {
			m.byFirst[first] = append(m.byFirst[first], i)
		} else {
			m.anyFirst = append(m.anyFirst, i)
		}
	}

	return m
}

// Match returns the last entry whose pattern matches rel, and false when no pattern matches it.
// rel is a path like the path of Pattern.Match.
func (m *Matcher) Match(rel string) (Entry, bool) {
	first, _, _ := strings.Cut(rel, "/")

	// Try the candidates from the last entry to the first one.
	byFirst, anyFirst := m.byFirst[first], m.anyFirst
	for len(byFirst) > 0 || len(anyFirst) > 0 {
		var i int
		if len(anyFirst) == 0 || len(byFirst) > 0 && byFirst[len(byFirst)-1] > anyFirst[len(anyFirst)-1] {
			i, byFirst = byFirst[len(byFirst)-1], byFirst[:len(byFirst)-1]
		} else {
			i, anyFirst = anyFirst[len(anyFirst)-1], anyFirst[:len(anyFirst)-1]
		}

		if m.patterns[i].Match(rel) {
			return m.entries[i], true
		}
	}

	return Entry{}, false
}
