// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Tests for the File Exist check.
// Derived from github.com/mszostok/codeowners-validator (Apache License 2.0), with changes.

package check

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

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
