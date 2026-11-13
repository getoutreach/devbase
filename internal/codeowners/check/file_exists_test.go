// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Tests for the File Exist check.
// Derived from github.com/mszostok/codeowners-validator (Apache License 2.0), with changes.

package check

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/go-git/go-git/v5/plumbing/format/gitignore"
	"gotest.tools/v3/assert"
)

// newErrIssue returns the error issue that the check reports for the entry on line 2.
func newErrIssue(msg string) Issue {
	return Issue{Severity: Error, LineNo: 2, Message: msg}
}

// TestFileExists validates that file exists checker supports
// syntax used by CODEOWNERS. As the CODEOWNERS file uses a pattern that
// follows the same rules used in gitignore files, the test cases cover
// patterns from this document: https://git-scm.com/docs/gitignore#_pattern_format
func TestFileExists(t *testing.T) {
	tests := map[string]struct {
		codeownersInput string
		expectedIssues  []Issue
		paths           []string
	}{
		"Should found JS file": {
			codeownersInput: `
					*.js @pico
			`,
			paths: []string{
				"somewhere/over/the/rainbow/here/it/is.js",
				"somewhere/not/here/it/is.go",
			},
		},
		"Should match directory 'foo' anywhere": {
			codeownersInput: `
					**/foo @pico
			`,
			paths: []string{
				"somewhere/over/the/foo/here/it/is.js",
			},
		},
		"Should match file 'foo' anywhere": {
			codeownersInput: `
					**/foo.js @pico
			`,
			paths: []string{
				"somewhere/over/the/rainbow/here/it/foo.js",
			},
		},
		"Should match directory 'bar' anywhere that is directly under directory 'foo'": {
			codeownersInput: `
					**/foo/bar @bello
			`,
			paths: []string{
				"somewhere/over/the/foo/bar/it/is.js",
			},
		},
		"Should match file 'bar' anywhere that is directly under directory 'foo'": {
			codeownersInput: `
					**/foo/bar.js @bello
			`,
			paths: []string{
				"somewhere/over/the/foo/bar.js",
			},
		},
		"Should match all files inside directory 'abc'": {
			codeownersInput: `
					abc/** @bello
			`,
			paths: []string{
				"abc/over/the/rainbow/bar.js",
			},
		},
		"Should match 'a/b', 'a/x/b', 'a/x/y/b' and so on": {
			codeownersInput: `
					a/**/b @bello
			`,
			paths: []string{
				"a/somewhere/over/the/b/foo.js",
			},
		},
		"Should match a pattern rooted at the repository root": {
			codeownersInput: `
					/docs/ @bello
			`,
			paths: []string{
				"docs/readme.md",
			},
		},
		// https://github.community/t/codeowners-file-with-a-not-file-type-condition/1423
		"Should not match with negation pattern": {
			codeownersInput: `
					!/codeowners-validator @pico
			`,
			paths: []string{
				"somewhere/over/the/rainbow/here/it/is.js",
			},
			expectedIssues: []Issue{
				newErrIssue(`"!/codeowners-validator" does not match any files in repository`),
			},
		},
		"Should not found JS file": {
			codeownersInput: `
					*.js @pico
			`,
			expectedIssues: []Issue{
				newErrIssue(`"*.js" does not match any files in repository`),
			},
		},
		"Should not match directory 'foo' anywhere": {
			codeownersInput: `
					**/foo @pico
			`,
			expectedIssues: []Issue{
				newErrIssue(`"**/foo" does not match any files in repository`),
			},
		},
		"Should not match file 'foo' anywhere": {
			codeownersInput: `
					**/foo.js @pico
			`,
			expectedIssues: []Issue{
				newErrIssue(`"**/foo.js" does not match any files in repository`),
			},
		},
		"Should no match directory 'bar' anywhere that is directly under directory 'foo'": {
			codeownersInput: `
					**/foo/bar @bello
			`,
			expectedIssues: []Issue{
				newErrIssue(`"**/foo/bar" does not match any files in repository`),
			},
		},
		"Should not match file 'bar' anywhere that is directly under directory 'foo'": {
			codeownersInput: `
					**/foo/bar.js @bello
			`,
			expectedIssues: []Issue{
				newErrIssue(`"**/foo/bar.js" does not match any files in repository`),
			},
		},
		"Should not match all files inside directory 'abc'": {
			codeownersInput: `
					abc/** @bello
			`,
			expectedIssues: []Issue{
				newErrIssue(`"abc/**" does not match any files in repository`),
			},
		},
		"Should not match 'a/**/b'": {
			codeownersInput: `
					a/**/b @bello
			`,
			expectedIssues: []Issue{
				newErrIssue(`"a/**/b" does not match any files in repository`),
			},
		},
	}

	for tn, tc := range tests {
		t.Run(tn, func(t *testing.T) {
			in := loadInput(t, tc.codeownersInput)
			in.RepoDir = newGitRepo(t, tc.paths...)

			out, err := NewFileExist().Check(t.Context(), in)
			assert.NilError(t, err)
			assert.DeepEqual(t, out.Issues, tc.expectedIssues)
		})
	}
}

func TestFileExistsUntrackedFiles(t *testing.T) {
	in := loadInput(t, "*.go @pico\n*.log @pico\n*.txt @pico\n")
	in.RepoDir = newGitRepo(t, "tracked.go")

	// An untracked file counts, an ignored file does not.
	assert.NilError(t, os.WriteFile(filepath.Join(in.RepoDir, "untracked.txt"), nil, 0o600))
	assert.NilError(t, os.WriteFile(filepath.Join(in.RepoDir, ".gitignore"), []byte("*.log\n"), 0o600))
	assert.NilError(t, os.WriteFile(filepath.Join(in.RepoDir, "ignored.log"), nil, 0o600))

	out, err := NewFileExist().Check(t.Context(), in)
	assert.NilError(t, err)
	assert.DeepEqual(t, out.Issues, []Issue{
		{Severity: Error, LineNo: 2, Message: `"*.log" does not match any files in repository`},
	})
}

func TestFileExistsStaged(t *testing.T) {
	in := loadInput(t, "*.go @pico\n*.txt @pico\n")
	in.RepoDir = newGitRepo(t, "tracked.go")
	in.Staged = true

	// An untracked file does not count.
	assert.NilError(t, os.WriteFile(filepath.Join(in.RepoDir, "untracked.txt"), nil, 0o600))

	out, err := NewFileExist().Check(t.Context(), in)
	assert.NilError(t, err)
	assert.DeepEqual(t, out.Issues, []Issue{
		{Severity: Error, LineNo: 2, Message: `"*.txt" does not match any files in repository`},
	})
}

func TestFileExistsNotAGitRepository(t *testing.T) {
	in := loadInput(t, "* @pico")
	in.RepoDir = t.TempDir()

	out, err := NewFileExist().Check(t.Context(), in)
	assert.Assert(t, err != nil)
	assert.Assert(t, strings.Contains(err.Error(), "not a git repository"), "got %v", err)
	assert.Equal(t, len(out.Issues), 0)
}

func TestFileExistsSubmodule(t *testing.T) {
	for _, staged := range []bool{false, true} {
		t.Run(fmt.Sprintf("staged=%t", staged), func(t *testing.T) {
			in := loadInput(t, "/vendor/lib/ @pico\n/vendor/lib/x.go @pico\n")
			in.RepoDir = newGitRepo(t, "a.go")
			in.Staged = staged

			// A submodule that is not checked out is a gitlink in the index and an empty directory.
			runGit(t, in.RepoDir, "update-index", "--add", "--cacheinfo", "160000,"+strings.Repeat("1", 40)+",vendor/lib")
			assert.NilError(t, os.MkdirAll(filepath.Join(in.RepoDir, "vendor", "lib"), 0o755))

			out, err := NewFileExist().Check(t.Context(), in)
			assert.NilError(t, err)
			assert.DeepEqual(t, out.Issues, []Issue{
				{Severity: Error, LineNo: 2, Message: `"/vendor/lib/x.go" does not match any files in repository`},
			})
		})
	}
}

func TestFileExistsDeletedFile(t *testing.T) {
	tests := map[string]struct {
		staged bool
		issues []Issue
	}{
		"the working tree": {
			issues: []Issue{{Severity: Error, LineNo: 1, Message: `"/old/" does not match any files in repository`}},
		},
		"the git index": {staged: true},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			in := loadInput(t, "/old/ @pico\n/a.go @pico\n")
			in.RepoDir = newGitRepo(t, "a.go", "old/x")
			in.Staged = tc.staged

			// The deletion is not staged.
			assert.NilError(t, os.Remove(filepath.Join(in.RepoDir, "old", "x")))

			out, err := NewFileExist().Check(t.Context(), in)
			assert.NilError(t, err)
			assert.DeepEqual(t, out.Issues, tc.issues)
		})
	}
}

func TestAnchoredTop(t *testing.T) {
	tests := map[string]string{
		"/docs/":          "docs",
		"/docs":           "docs",
		"docs/api":        "docs",
		"/docs/**/v1.md":  "docs",
		"//docs/x":        "docs",
		"!/docs/x":        "docs",
		"docs/api/v1.md ": "docs",
		"docs/":           "",
		"docs":            "",
		"*.go":            "",
		"**/api":          "",
		"/**/x":           "",
		"/d*/x":           "",
		"/d?/x":           "",
		"/[d]ocs/x":       "",
		`/\[d]/x`:         "",
		"a/ ":             "",
		"/":               "",
	}
	for pattern, want := range tests {
		t.Run(pattern, func(t *testing.T) {
			top, ok := anchoredTop(pattern)
			assert.Equal(t, top, want)
			assert.Equal(t, ok, want != "")
		})
	}
}

// TestFileIndexMatchesFullScan checks that the lookup by the first element of the path finds the
// same patterns as a match against every file.
func TestFileIndexMatchesFullScan(t *testing.T) {
	paths := []string{
		"a.go", "docs/readme.md", "docs/api/v1.md", "src/app/main.go", "src/lib/util.go",
		"build/logs/x.log", "foo/bar/baz.js", "a/x/y/b/c.txt", "with space/x", "[weird]/f", "star*/f",
		"q?/f", ".github/CODEOWNERS", "deep/a/b/c/d/e.go", "embedded/",
	}
	files := make([]repoFile, 0, len(paths)+1)
	for _, path := range paths {
		files = append(files, repoFile{path: strings.Split(path, "/")})
	}
	files = append(files, repoFile{path: []string{"vendor", "lib"}, isDir: true})

	patterns := []string{
		"*", "**", "/**", "/", "*.go", "a.go", "/a.go", "/a.go/", "a.go/", "/docs/", "/docs", "docs/", "docs/*",
		"/docs/*.md", "docs/**", "/docs/**/v1.md", "**/api", "**/api/", "/docs//api", "//docs", "docs/api/v1.md ",
		"/src/app/", "src/*/main.go", "/src/*/", "/src/app/main.go/", "/vendor/lib/", "vendor/lib/", "/vendor/lib",
		"/vendor/lib/x", "lib/", "/build/logs/", "/build/logs", "build/logs/x.log", "foo/bar", "/foo/bar/",
		"foo/bar/baz.js/", "a/**/b", "a/**/c.txt", "/a/x/", "/nope/", "nope/x", "/[weird]/f", `/\[weird\]/f`,
		"/star*/f", `/star\*/f`, "/q?/f", "/with space/x", "!/docs/", "!docs/x", "/.github/", ".github/CODEOWNERS",
		"/deep/a/", "deep/a/b/c/d/e.go", "/deep/a/b/c/d/e.go", "/embedded/", "/embedded", "a/ ", "x/ ", "/**/e.go",
		"**/b/c.txt", "/deep/**", "/deep/**/e.go/", "/s*/app/", "/d?cs/", "/[ds]ocs/readme.md", `/\docs/`,
	}

	index := newFileIndex(files)
	indexed := 0
	for _, p := range patterns {
		t.Run(p, func(t *testing.T) {
			pattern := gitignore.ParsePattern(p, nil)
			want := slices.ContainsFunc(files, func(f repoFile) bool {
				return pattern.Match(f.path, f.isDir) == gitignore.Exclude
			})
			assert.Equal(t, index.match(p), want)
		})
		if _, ok := anchoredTop(p); ok {
			indexed++
		}
	}
	assert.Assert(t, indexed >= 30, "only %d patterns use the index", indexed)
}

// BenchmarkFileExist runs the files check on 50,000 files in the git index with 500 patterns. Most
// patterns are anchored at the root, like in a typical CODEOWNERS file.
func BenchmarkFileExist(b *testing.B) {
	dir := b.TempDir()
	runGit(b, dir, "init", "-q")

	// The files are only in the git index, and all have the empty content.
	blob, err := gitCommand(b, dir, "hash-object", "-w", "--stdin").Output()
	assert.NilError(b, err)

	var index strings.Builder
	for i := range 50_000 {
		fmt.Fprintf(&index, "100644 %s\tdir%03d/pkg%02d/file%02d.go\n", bytes.TrimSpace(blob), i/500, i/50%10, i%50)
	}
	update := gitCommand(b, dir, "update-index", "--index-info")
	update.Stdin = strings.NewReader(index.String())
	out, err := update.CombinedOutput()
	assert.NilError(b, err, "git update-index: %s", out)

	var codeowners strings.Builder
	for i := range 500 {
		switch {
		case i < 350: // An anchored pattern that matches.
			fmt.Fprintf(&codeowners, "/dir%03d/pkg%02d/ @team\n", i%100, i%10)
		case i < 450: // An anchored pattern that matches no file.
			fmt.Fprintf(&codeowners, "/dir%03d/missing%02d/ @team\n", i%100, i%10)
		case i < 475: // A pattern that is not anchored and matches.
			fmt.Fprintf(&codeowners, "file%02d.go @team\n", i%50)
		default: // A pattern that is not anchored and matches no file.
			fmt.Fprintf(&codeowners, "*.txt%02d @team\n", i%50)
		}
	}

	in := loadInput(b, codeowners.String())
	in.RepoDir = dir
	in.Staged = true

	for b.Loop() {
		out, err := NewFileExist().Check(b.Context(), in)
		if err != nil {
			b.Fatal(err)
		}
		if len(out.Issues) != 125 {
			b.Fatalf("got %d issues, want 125", len(out.Issues))
		}
	}
}
