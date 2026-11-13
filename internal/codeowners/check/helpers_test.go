// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Shared helpers for the check package tests.
// Derived from github.com/mszostok/codeowners-validator (Apache License 2.0), with changes.

package check

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/getoutreach/devbase/v2/internal/codeowners"
	"gotest.tools/v3/assert"
)

// fixtureValidCodeowners is a CODEOWNERS file that every check accepts.
const fixtureValidCodeowners = `
		# These owners will be the default owners for everything
		*       @global-owner1 @global-owner2

		# js owner
		*.js    @js-owner

		*.go docs@example.com

		/build/logs/ @doctocat

		/script m.t@g.com
`

// loadInput returns an Input with the entries parsed from a CODEOWNERS file.
func loadInput(t *testing.T, content string) Input {
	t.Helper()

	entries, err := codeowners.Parse(strings.NewReader(content))
	assert.NilError(t, err)

	return Input{Entries: entries}
}

// newGitRepo creates a git repository in a temporary directory, with an empty file for each path,
// all staged. It returns the directory.
func newGitRepo(t *testing.T, files ...string) string {
	t.Helper()

	dir := t.TempDir()
	runGit(t, dir, "init", "-q")

	for _, f := range files {
		p := filepath.Join(dir, f)
		assert.NilError(t, os.MkdirAll(filepath.Dir(p), 0o755))
		assert.NilError(t, os.WriteFile(p, []byte("hakuna-matata"), 0o600))
	}
	runGit(t, dir, "add", "-A")

	return dir
}

// gitCommand returns the command that runs git with args in dir, with an identity for commits
// and merges.
func gitCommand(t *testing.T, dir string, args ...string) *exec.Cmd {
	t.Helper()

	identity := []string{"-c", "user.name=test", "-c", "user.email=test@example.com"}
	cmd := exec.CommandContext(t.Context(), "git", append(identity, args...)...)
	cmd.Dir = dir

	return cmd
}

// runGit runs git with args in dir and fails the test on error.
func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()

	out, err := gitCommand(t, dir, args...).CombinedOutput()
	assert.NilError(t, err, "git %v: %s", args, out)
}

// commitAll commits the changes of the tracked files in dir.
func commitAll(t *testing.T, dir, msg string) {
	t.Helper()

	runGit(t, dir, "commit", "-q", "-a", "-m", msg)
}

// assertSingleIssue asserts that issues has exactly one issue, equal to want, or none when want is nil.
func assertSingleIssue(t *testing.T, want *Issue, got []Issue) {
	t.Helper()

	if want == nil {
		assert.Equal(t, len(got), 0, "unexpected issues: %v", got)
		return
	}

	assert.Equal(t, len(got), 1, "issues: %v", got)
	assert.DeepEqual(t, *want, got[0])
}
