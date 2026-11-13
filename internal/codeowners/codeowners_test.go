// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Tests for the CODEOWNERS file parser and loader.
// Derived from github.com/mszostok/codeowners-validator (Apache License 2.0), with changes.

package codeowners_test

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/getoutreach/devbase/v2/internal/codeowners"
	"gotest.tools/v3/assert"
)

// sampleCodeownersFile is a CODEOWNERS file that covers comments and blank lines.
const sampleCodeownersFile = `
# Sample codeowner file
*	@everyone

src/**	@org/hakuna-matata @pico-bello
pkg/github.com/**	@myk
tests/**	@ghost # some comment
internal/**	@ghost #some comment v2

`

// writeFile creates a file, and its parent directories, below root.
func writeFile(t *testing.T, root, rel, content string) {
	t.Helper()

	p := filepath.Join(root, rel)
	assert.NilError(t, os.MkdirAll(filepath.Dir(p), 0o755))
	assert.NilError(t, os.WriteFile(p, []byte(content), 0o600))
}

func TestParseCodeownersSuccess(t *testing.T) {
	expEntries := []codeowners.Entry{
		{LineNo: 3, Pattern: "*", Owners: []string{"@everyone"}},
		{LineNo: 5, Pattern: "src/**", Owners: []string{"@org/hakuna-matata", "@pico-bello"}},
		{LineNo: 6, Pattern: "pkg/github.com/**", Owners: []string{"@myk"}},
		{LineNo: 7, Pattern: "tests/**", Owners: []string{"@ghost"}},
		{LineNo: 8, Pattern: "internal/**", Owners: []string{"@ghost"}},
	}

	repo := t.TempDir()
	writeFile(t, repo, "CODEOWNERS", sampleCodeownersFile)

	entries, err := codeowners.Load(repo)
	assert.NilError(t, err)
	assert.DeepEqual(t, entries, expEntries)
}

func TestParseInlineComment(t *testing.T) {
	entries, err := codeowners.Parse(strings.NewReader("* @a # x #y\n"))
	assert.NilError(t, err)
	assert.DeepEqual(t, entries, []codeowners.Entry{
		{LineNo: 1, Pattern: "*", Owners: []string{"@a"}},
	})
}

func TestParseByteOrderMark(t *testing.T) {
	tests := map[string]struct {
		content string
		want    []codeowners.Entry
	}{
		"before a comment line": {
			content: "\uFEFF# comment\n* @a\n",
			want:    []codeowners.Entry{{LineNo: 2, Pattern: "*", Owners: []string{"@a"}}},
		},
		"before an entry line": {
			content: "\uFEFF* @a\n",
			want:    []codeowners.Entry{{LineNo: 1, Pattern: "*", Owners: []string{"@a"}}},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			entries, err := codeowners.Parse(strings.NewReader(tc.content))
			assert.NilError(t, err)
			assert.DeepEqual(t, entries, tc.want)
		})
	}
}

func TestParseReaderError(t *testing.T) {
	_, err := codeowners.Parse(strings.NewReader(strings.Repeat("a", 1<<20)))
	assert.ErrorContains(t, err, "token too long")
}

func TestEntryString(t *testing.T) {
	e := codeowners.Entry{LineNo: 7, Pattern: "docs/*", Owners: []string{"@a", "@b"}}
	assert.Equal(t, e.String(), "line 7: docs/*\t@a, @b")
}

func TestFindCodeownersFileSuccess(t *testing.T) {
	tests := map[string]struct {
		location string
		suffix   string
	}{
		"Should find the CODEOWNERS at root":          {location: "."},
		"Should find the CODEOWNERS in docs/":         {location: "docs"},
		"Should find the CODEOWNERS IN .github":       {location: ".github"},
		"Should manage situation with trailing slash": {location: ".github", suffix: "/"},
	}
	for tn, tc := range tests {
		t.Run(tn, func(t *testing.T) {
			repo := t.TempDir()
			writeFile(t, repo, filepath.Join(tc.location, "CODEOWNERS"), "")

			entries, err := codeowners.Load(repo + tc.suffix)
			assert.NilError(t, err)
			assert.Equal(t, len(entries), 0)
		})
	}
}

func TestFindCodeownersFileFailure(t *testing.T) {
	repo := t.TempDir() + "/"
	expErrMsg := "No CODEOWNERS found in the root, docs/, or .github/ directory of the repository " + repo

	entries, err := codeowners.Load(repo)
	assert.Error(t, err, expErrMsg)
	assert.Assert(t, errors.Is(err, codeowners.ErrNotFound))
	assert.Assert(t, entries == nil)
}

func TestMultipleCodeownersFileFailure(t *testing.T) {
	tests := map[string]struct {
		expLocations string
		locations    []string
	}{
		"Should report that CODEOWNERS file was found on root and docs/": {
			expLocations: "./CODEOWNERS and ./docs/CODEOWNERS",
			locations:    []string{"CODEOWNERS", filepath.Join("docs", "CODEOWNERS")},
		},
		"Should report that CODEOWNERS file was found on root and .github/": {
			expLocations: "./CODEOWNERS and ./.github/CODEOWNERS",
			locations:    []string{"CODEOWNERS", filepath.Join(".github", "CODEOWNERS")},
		},
		"Should report that CODEOWNERS file was found in docs/ and .github/": {
			expLocations: "./docs/CODEOWNERS and ./.github/CODEOWNERS",
			locations:    []string{filepath.Join(".github", "CODEOWNERS"), filepath.Join("docs", "CODEOWNERS")},
		},
		"Should report that CODEOWNERS file was found on root, docs/ and .github/": {
			expLocations: "./CODEOWNERS, ./docs/CODEOWNERS, and ./.github/CODEOWNERS",
			locations: []string{
				"CODEOWNERS", filepath.Join(".github", "CODEOWNERS"), filepath.Join("docs", "CODEOWNERS"),
			},
		},
	}
	for tn, tc := range tests {
		t.Run(tn, func(t *testing.T) {
			repo := t.TempDir() + "/"
			for _, location := range tc.locations {
				writeFile(t, repo, location, "")
			}

			expErrMsg := fmt.Sprintf("Multiple CODEOWNERS files found in the %s locations of the repository %s",
				tc.expLocations, repo)

			entries, err := codeowners.Load(repo)
			assert.Error(t, err, expErrMsg)
			assert.Assert(t, errors.Is(err, codeowners.ErrMultipleFiles))
			assert.Assert(t, entries == nil)
		})
	}
}

func TestLoadIgnoresNonDirectoryLocation(t *testing.T) {
	repo := t.TempDir()
	writeFile(t, repo, "docs", "not a directory")
	writeFile(t, repo, "CODEOWNERS", "* @a\n")

	entries, err := codeowners.Load(repo)
	assert.NilError(t, err)
	assert.DeepEqual(t, entries, []codeowners.Entry{{LineNo: 1, Pattern: "*", Owners: []string{"@a"}}})
}

func TestRead(t *testing.T) {
	for _, rel := range []string{"CODEOWNERS", "docs/CODEOWNERS", ".github/CODEOWNERS"} {
		t.Run(rel, func(t *testing.T) {
			repo := t.TempDir()
			writeFile(t, repo, rel, "* @a\n")

			path, content, err := codeowners.Read(repo)
			assert.NilError(t, err)
			assert.Equal(t, path, rel)
			assert.Equal(t, string(content), "* @a\n")
		})
	}

	t.Run("no file", func(t *testing.T) {
		_, _, err := codeowners.Read(t.TempDir())
		assert.Assert(t, errors.Is(err, codeowners.ErrNotFound), "got %v", err)
	})
}
