// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Tests for the not owned file check.
// Derived from github.com/mszostok/codeowners-validator (Apache License 2.0), with changes.

package check

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"gotest.tools/v3/assert"
)

// gitOutput runs git with args in dir and returns its standard output.
func gitOutput(t *testing.T, dir string, args ...string) string {
	t.Helper()

	cmd := exec.CommandContext(t.Context(), "git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	assert.NilError(t, err, "git %v", args)

	return string(out)
}

func TestNotOwnedFile(t *testing.T) {
	tests := []struct {
		name       string
		codeowners string
		files      []string
		cfg        NotOwnedFileConfig
		wantMsg    string
	}{
		{
			name:       "all files are owned",
			codeowners: "*.go @owner\n/docs/ @docs",
			files:      []string{"a.go", "sub/b.go", "docs/c.md"},
		},
		{
			name:       "an unowned file is reported",
			codeowners: "*.go @owner",
			files:      []string{"a.go", "b.txt"},
			wantMsg: `Found 1 not owned files (skipped patterns: ""):
            * b.txt`,
		},
		{
			name:       "unowned files are reported in the order of git",
			codeowners: "*.go @owner",
			files:      []string{"z.txt", "a.txt", "sub/m.txt", "a.go"},
			wantMsg: `Found 3 not owned files (skipped patterns: ""):
            * a.txt
            * sub/m.txt
            * z.txt`,
		},
		{
			name:       "a skipped pattern reports the files that only it owns",
			codeowners: "* @global\n*.go @owner",
			files:      []string{"a.go", "b.txt"},
			cfg:        NotOwnedFileConfig{SkipPatterns: []string{"*"}},
			wantMsg: `Found 1 not owned files (skipped patterns: "*"):
            * b.txt`,
		},
		{
			name:       "skipped patterns are sorted and unique in the message",
			codeowners: "*.go @owner\n*.md @docs\n*.txt @text",
			files:      []string{"a.go", "b.md", "c.txt", "d.yml"},
			cfg:        NotOwnedFileConfig{SkipPatterns: []string{"*.txt", "*.md", "*.txt"}},
			wantMsg: `Found 3 not owned files (skipped patterns: "*.md,*.txt"):
            * b.md
            * c.txt
            * d.yml`,
		},
		{
			name:       "subdirectories limit the files",
			codeowners: "*.go @owner",
			files:      []string{"a.txt", "one/b.txt", "one/c.go", "two/d.txt"},
			cfg:        NotOwnedFileConfig{Subdirectories: []string{"one", "two"}},
			wantMsg: `Found 2 not owned files (skipped patterns: ""):
            * one/b.txt
            * two/d.txt`,
		},
		{
			name:       "a negated pattern is ignored",
			codeowners: "*.txt @owner\n!b.txt @nobody",
			files:      []string{"a.txt", "b.txt"},
		},
		{
			name:       "an empty CODEOWNERS file owns nothing",
			codeowners: "# only a comment",
			files:      []string{"a.go"},
			wantMsg:    "The CODEOWNERS file is empty. The files in the repository don't have any owner.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := loadInput(t, tt.codeowners)
			in.RepoDir = newGitRepo(t, tt.files...)

			out, err := NewNotOwnedFile(tt.cfg).Check(t.Context(), in)
			assert.NilError(t, err)

			if tt.wantMsg == "" {
				assertSingleIssue(t, nil, out.Issues)
				return
			}

			assertSingleIssue(t, &Issue{Severity: Error, Message: tt.wantMsg}, out.Issues)
		})
	}
}

func TestNotOwnedFileDoesNotChangeRepo(t *testing.T) {
	in := loadInput(t, "*.go @owner\n.gitignore @owner")
	in.RepoDir = newGitRepo(t, "a.go", "b.go", ".gitignore")

	gitignore := filepath.Join(in.RepoDir, ".gitignore")
	assert.NilError(t, os.WriteFile(gitignore, []byte("ignored.log\n"), 0o600))
	runGit(t, in.RepoDir, "add", "-A")
	runGit(t, in.RepoDir, "-c", "user.name=test", "-c", "user.email=test@example.com", "commit", "-q", "-m", "init")

	// Make the repository dirty: an unstaged edit of a tracked file, an untracked file that no
	// pattern matches, and a staged file that no pattern matches.
	assert.NilError(t, os.WriteFile(filepath.Join(in.RepoDir, "a.go"), []byte("edited"), 0o600))
	assert.NilError(t, os.WriteFile(filepath.Join(in.RepoDir, "untracked.txt"), nil, 0o600))
	assert.NilError(t, os.WriteFile(filepath.Join(in.RepoDir, "staged.txt"), nil, 0o600))
	runGit(t, in.RepoDir, "add", "staged.txt")

	statusBefore := gitOutput(t, in.RepoDir, "status", "--porcelain")
	assert.Assert(t, statusBefore != "")

	out, err := NewNotOwnedFile(NotOwnedFileConfig{}).Check(t.Context(), in)
	assert.NilError(t, err)

	want := &Issue{Severity: Error, Message: `Found 1 not owned files (skipped patterns: ""):
            * staged.txt`}
	assertSingleIssue(t, want, out.Issues)

	assert.Equal(t, gitOutput(t, in.RepoDir, "status", "--porcelain"), statusBefore)

	got, err := os.ReadFile(gitignore)
	assert.NilError(t, err)
	assert.Equal(t, string(got), "ignored.log\n")

	content, err := os.ReadFile(filepath.Join(in.RepoDir, "a.go"))
	assert.NilError(t, err)
	assert.Equal(t, string(content), "edited")
}

func TestNotOwnedFileNotAGitRepo(t *testing.T) {
	in := loadInput(t, "*.go @owner")
	in.RepoDir = t.TempDir()

	_, err := NewNotOwnedFile(NotOwnedFileConfig{}).Check(t.Context(), in)
	assert.ErrorContains(t, err, "not a git repository")
}
