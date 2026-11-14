// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Tests for the File Exist check.
// Derived from github.com/mszostok/codeowners-validator (Apache License 2.0), with changes.

package check

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/getoutreach/devbase/v2/internal/codeowners"
	"gotest.tools/v3/assert"
)

// newErrIssue returns the error issue that the check reports for the entry on line 2.
func newErrIssue(msg string) Issue {
	return Issue{Severity: Error, LineNo: 2, Message: msg}
}

// TestFileExists validates that file exists checker supports
// syntax used by CODEOWNERS. As the CODEOWNERS file uses a pattern that
// follows most of the rules used in gitignore files, the test cases cover
// patterns from this document: https://git-scm.com/docs/gitignore#_pattern_format
// and the rules where GitHub differs from it.
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
		// Negation does not work. See "CODEOWNERS syntax" in:
		// https://docs.github.com/en/repositories/managing-your-repositorys-settings-and-features/customizing-your-repository/about-code-owners
		"Should not match with negation pattern": {
			codeownersInput: `
					!/codeowners-validator @pico
			`,
			paths: []string{
				"codeowners-validator",
				"somewhere/over/the/rainbow/here/it/is.js",
			},
			expectedIssues: []Issue{
				newErrIssue(`"!/codeowners-validator" does not match any files in repository`),
			},
		},
		"Should match a file in the directory of a pattern with a wildcard at the end": {
			codeownersInput: `
					docs/* @pico
			`,
			paths: []string{
				"docs/getting-started.md",
			},
		},
		"Should not match a nested file with a wildcard at the end": {
			codeownersInput: `
					docs/* @pico
			`,
			paths: []string{
				"docs/build-app/troubleshooting.md",
			},
			expectedIssues: []Issue{
				newErrIssue(`"docs/*" does not match any files in repository`),
			},
		},
		"Should match brackets literally": {
			codeownersInput: `
					/pages/[id].tsx @pico
			`,
			paths: []string{
				"pages/[id].tsx",
			},
		},
		"Should not match a character range": {
			codeownersInput: `
					[ab].go @pico
			`,
			paths: []string{
				"a.go",
			},
			expectedIssues: []Issue{
				newErrIssue(`"[ab].go" does not match any files in repository`),
			},
		},
		"Should match '!' literally": {
			codeownersInput: `
					!important.txt @pico
			`,
			paths: []string{
				"!important.txt",
			},
		},
		"Should not match the directory itself with a trailing '/**'": {
			codeownersInput: `
					abc/** @pico
			`,
			paths: []string{
				"abc",
			},
			expectedIssues: []Issue{
				newErrIssue(`"abc/**" does not match any files in repository`),
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

// TestFileIndexMatchesFullScan checks that the lookup by the first segment of the path finds the
// same patterns as a match against every file.
func TestFileIndexMatchesFullScan(t *testing.T) {
	paths := []string{
		"a.go", "docs/readme.md", "docs/api/v1.md", "src/app/main.go", "src/lib/util.go",
		"build/logs/x.log", "foo/bar/baz.js", "a/x/y/b/c.txt", "with space/x", "[weird]/f", "star*/f",
		"q?/f", ".github/CODEOWNERS", "deep/a/b/c/d/e.go", "embedded/", "vendor/lib/", "!bang/f", `\#hash/f`,
	}

	patterns := []string{
		"*", "**", "/**", "/", "*.go", "a.go", "/a.go", "/a.go/", "a.go/", "/docs/", "/docs", "docs/", "docs/*",
		"/docs/*.md", "docs/**", "/docs/**/v1.md", "**/api", "**/api/", "/docs//api", "//docs", "/src/app/",
		"src/*/main.go", "/src/*/", "/src/app/main.go/", "/vendor/lib/", "vendor/lib/", "/vendor/lib",
		"/vendor/lib/x", "lib/", "/build/logs/", "/build/logs", "build/logs/x.log", "foo/bar", "/foo/bar/",
		"foo/bar/baz.js/", "a/**/b", "a/**/c.txt", "/a/x/", "/nope/", "nope/x", "/[weird]/f", "/star*/f",
		"/q?/f", "/with space/x", "!/docs/", "!bang/f", "/!bang/", `\#hash/f`, "/.github/", ".github/CODEOWNERS",
		"/deep/a/", "deep/a/b/c/d/e.go", "/deep/a/b/c/d/e.go", "/embedded/", "/embedded", "/**/e.go",
		"**/b/c.txt", "/deep/**", "/deep/**/e.go/", "/s*/app/", "/d?cs/", "/[ds]ocs/readme.md",
	}

	index := newFileIndex(paths)
	indexed := 0
	for _, p := range patterns {
		pattern := codeowners.CompilePattern(p)
		t.Run(p, func(t *testing.T) {
			assert.Equal(t, index.match(pattern), slices.ContainsFunc(paths, pattern.Match))
		})
		if _, ok := pattern.FirstSegment(); ok {
			indexed++
		}
	}
	assert.Assert(t, indexed >= 30, "only %d patterns use the index", indexed)
}

// BenchmarkFileExist runs the files check on the benchmark repository.
func BenchmarkFileExist(b *testing.B) {
	in := loadInput(b, benchCodeowners())
	in.RepoDir = newBenchRepo(b)
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
