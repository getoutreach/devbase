// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Tests for matching paths against CODEOWNERS patterns.

package codeowners_test

import (
	"strings"
	"testing"

	"github.com/getoutreach/devbase/v2/internal/codeowners"
	"gotest.tools/v3/assert"
)

func TestPatternMatch(t *testing.T) {
	tests := []struct {
		pattern string
		match   []string
		noMatch []string
	}{
		// The examples of the GitHub documentation.
		{pattern: "*", match: []string{"a.go", "docs/a.md", "deep/x/y/z", "vendor/lib/"}},
		{pattern: "*.js", match: []string{"a.js", "src/x/a.js"}, noMatch: []string{"a.go", "a.jsx", "a.js/x.go"}},
		{
			pattern: "/build/logs/",
			match:   []string{"build/logs/a.log", "build/logs/sub/b.log", "build/logs/"},
			noMatch: []string{"build/logs", "x/build/logs/a.log", "build/logs2/a.log"},
		},
		{
			pattern: "docs/*",
			match:   []string{"docs/getting-started.md", "docs/lib/"},
			noMatch: []string{"docs/build-app/troubleshooting.md", "x/docs/a.md", "docs"},
		},
		{pattern: "apps/", match: []string{"apps/a", "x/apps/a", "x/y/apps/z/w", "apps/"}, noMatch: []string{"apps", "myapps/a"}},
		{pattern: "/docs/", match: []string{"docs/a.md", "docs/x/y.md"}, noMatch: []string{"x/docs/a.md", "docs"}},
		{
			pattern: "**/logs",
			match:   []string{"build/logs/a", "scripts/logs/a", "deeply/nested/logs/a", "logs/a", "logs"},
			noMatch: []string{"build/logs2/a", "build/mylogs/a"},
		},
		{pattern: "/apps/github", match: []string{"apps/github", "apps/github/x.go"}, noMatch: []string{"apps/a.go", "x/apps/github"}},

		// "*" and "?" do not match "/".
		{pattern: "a*b", match: []string{"ab", "axyb", "x/ab"}, noMatch: []string{"a/b", "axb/c"}},
		{pattern: "/?.go", match: []string{"a.go", "é.go"}, noMatch: []string{"ab.go", ".go", "a/.go"}},
		{pattern: "a?b", match: []string{"axb"}, noMatch: []string{"a/b", "ab"}},
		{pattern: "a*b*c", match: []string{"abc", "axbyc", "abbc", "abcbc"}, noMatch: []string{"acb", "ab", "abcx"}},
		{pattern: "ab*b", match: []string{"abb", "abxb"}, noMatch: []string{"ab"}},

		// "**" as a leading, trailing, or middle segment.
		{pattern: "**/foo", match: []string{"foo", "a/foo", "a/b/foo/x"}, noMatch: []string{"foox", "a/xfoo"}},
		{pattern: "abc/**", match: []string{"abc/x", "abc/x/y"}, noMatch: []string{"abc", "abc/", "x/abc/y"}},
		{pattern: "abc/**/", match: []string{"abc/x/y", "abc/x/"}, noMatch: []string{"abc/x"}},
		{pattern: "a/**/b", match: []string{"a/b", "a/x/b", "a/x/y/b", "a/x/b/c"}, noMatch: []string{"a/xb", "x/a/b"}},
		{pattern: "a/**/**/b", match: []string{"a/b", "a/x/y/b"}, noMatch: []string{"a/c"}},
		{pattern: "**", match: []string{"a", "a/b"}},
		{pattern: "/**", match: []string{"a", "a/b"}},
		{pattern: "**/", match: []string{"a/b", "a/"}, noMatch: []string{"a"}},
		{pattern: "foo**bar", match: []string{"foobar", "fooxbar", "x/fooxybar"}, noMatch: []string{"foo/bar"}},

		// A "/" at the start or in the middle anchors the pattern at the root.
		{pattern: "/a.go", match: []string{"a.go"}, noMatch: []string{"x/a.go"}},
		{pattern: "a/b.go", match: []string{"a/b.go"}, noMatch: []string{"x/a/b.go"}},
		{pattern: "b.go", match: []string{"b.go", "x/a/b.go"}, noMatch: []string{"ab.go"}},

		// A pattern without a trailing "/" also matches the files below a directory.
		{pattern: "/docs", match: []string{"docs", "docs/x/y"}, noMatch: []string{"x/docs/y"}},
		{pattern: "logs", match: []string{"logs", "a/logs/b", "a/logs/"}, noMatch: []string{"a/logs2/b"}},
		{pattern: "src/*/main.go", match: []string{"src/app/main.go", "src/app/main.go/x"}, noMatch: []string{"src/main.go"}},

		// A pattern with a wildcard in its last segment only matches whole paths.
		{pattern: "docs/*.md", match: []string{"docs/a.md"}, noMatch: []string{"docs/a.md/x", "docs/x/a.md"}},
		{pattern: "/doc?", match: []string{"docs", "docs/"}, noMatch: []string{"docs/a.md"}},
		{pattern: "/a*", match: []string{"ab"}, noMatch: []string{"a/b"}},
		{pattern: "docs/**/*.md", match: []string{"docs/a.md", "docs/x/y/a.md"}, noMatch: []string{"docs/a.md/x"}},

		// The exception does not apply to a pattern that ends with "/".
		{pattern: "docs/*/", match: []string{"docs/a/b.md", "docs/a/b/c.md", "docs/a/"}, noMatch: []string{"docs/c.md"}},

		// GitHub does not support negation, character ranges, and escapes, so these characters
		// only match themselves.
		{pattern: "!a.go", match: []string{"!a.go", "x/!a.go"}, noMatch: []string{"a.go", "b.go"}},
		{pattern: "[ab].go", match: []string{"[ab].go"}, noMatch: []string{"a.go"}},
		{pattern: "/pages/[id].tsx", match: []string{"pages/[id].tsx"}, noMatch: []string{"pages/i.tsx"}},
		{pattern: "*[id]*.tsx", match: []string{"a[id].tsx", "[id]b.tsx"}, noMatch: []string{"ai.tsx"}},
		{pattern: "?]", match: []string{"a]"}, noMatch: []string{"a"}},
		{pattern: "?[id].tsx", match: []string{"a[id].tsx"}, noMatch: []string{"ai.tsx"}},
		{pattern: `?\x`, match: []string{`a\x`}, noMatch: []string{"ax"}},
		{pattern: `\#a`, match: []string{`\#a`}, noMatch: []string{"#a"}},
		{pattern: `a\*b`, match: []string{`a\xb`}, noMatch: []string{"a*b", "axb"}},

		// Matching is case-sensitive.
		{pattern: "/Docs/", match: []string{"Docs/a"}, noMatch: []string{"docs/a"}},
		{pattern: "*.MD", match: []string{"a.MD"}, noMatch: []string{"a.md"}},

		// A pattern without segments matches nothing.
		{pattern: "", noMatch: []string{"a", "a/b"}},
		{pattern: "/", noMatch: []string{"a", "a/b", "a/"}},
	}
	for _, tc := range tests {
		t.Run(tc.pattern, func(t *testing.T) {
			p := codeowners.CompilePattern(tc.pattern)
			for _, path := range tc.match {
				assert.Assert(t, p.Match(path), "%q does not match %q", tc.pattern, path)
			}
			for _, path := range tc.noMatch {
				assert.Assert(t, !p.Match(path), "%q matches %q", tc.pattern, path)
			}
		})
	}
}

func TestPatternFirstSegment(t *testing.T) {
	tests := map[string]string{
		"/docs/":         "docs",
		"/docs":          "docs",
		"docs/api":       "docs",
		"/docs/**/v1.md": "docs",
		"/[d]ocs/x":      "[d]ocs",
		"!/docs/x":       "!",
		"docs/":          "",
		"docs":           "",
		"*.go":           "",
		"**/api":         "",
		"/**/x":          "",
		"/d*/x":          "",
		"/d?/x":          "",
		"/":              "",
		"":               "",
	}
	for pattern, want := range tests {
		t.Run(pattern, func(t *testing.T) {
			first, ok := codeowners.CompilePattern(pattern).FirstSegment()
			assert.Equal(t, first, want)
			assert.Equal(t, ok, want != "")
		})
	}
}

func TestMatcher(t *testing.T) {
	entries, err := codeowners.Parse(strings.NewReader(`
* @default
/a/ @a
*.go @go
/a/x.go @ax
/b/ @b
/apps/ @octocat
/apps/github
`))
	assert.NilError(t, err)

	m := codeowners.NewMatcher(entries)
	tests := map[string]uint64{
		"a/x.go":          5,
		"a/y.go":          4,
		"a/z.txt":         3,
		"b/y.go":          6,
		"c/y.go":          4,
		"c/y.txt":         2,
		"apps/a.txt":      7,
		"apps/github/x":   8,
		"apps/github/x.y": 8,
	}
	for path, line := range tests {
		t.Run(path, func(t *testing.T) {
			entry, ok := m.Match(path)
			assert.Assert(t, ok)
			assert.Equal(t, entry.LineNo, line)
		})
	}

	t.Run("no match", func(t *testing.T) {
		m := codeowners.NewMatcher(entries[1:3])

		_, ok := m.Match("c/y.txt")
		assert.Assert(t, !ok)
	})
}
