// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Shared helpers for the check package tests.
// Derived from github.com/mszostok/codeowners-validator (Apache License 2.0), with changes.

package check

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/getoutreach/devbase/v2/internal/codeowners"
	"github.com/getoutreach/devbase/v2/internal/gittest"
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

// TestMain isolates the git commands of the tests from the git environment of the caller.
func TestMain(m *testing.M) {
	os.Exit(gittest.Run(m))
}

// loadInput returns an Input with the entries parsed from a CODEOWNERS file.
func loadInput(tb testing.TB, content string) Input {
	tb.Helper()

	entries, err := codeowners.Parse(strings.NewReader(content))
	assert.NilError(tb, err)

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
func gitCommand(tb testing.TB, dir string, args ...string) *exec.Cmd {
	tb.Helper()

	identity := []string{"-c", "user.name=test", "-c", "user.email=test@example.com"}
	cmd := exec.CommandContext(tb.Context(), "git", append(identity, args...)...)
	cmd.Dir = dir

	return cmd
}

// runGit runs git with args in dir and fails the test on error.
func runGit(tb testing.TB, dir string, args ...string) {
	tb.Helper()

	out, err := gitCommand(tb, dir, args...).CombinedOutput()
	assert.NilError(tb, err, "git %v: %s", args, out)
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

// newBenchRepo creates a git repository with 50,000 files in the git index and returns its
// directory. The files are dir000/pkg00/file00.go to dir099/pkg09/file49.go, and they are not in
// the working tree.
func newBenchRepo(b *testing.B) string {
	b.Helper()

	dir := b.TempDir()
	runGit(b, dir, "init", "-q")

	// All the files have the empty content.
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

	return dir
}

// benchCodeowners returns 500 CODEOWNERS entries for the repository of newBenchRepo. Most
// patterns are anchored at the root, like in a typical CODEOWNERS file. 125 patterns match no
// file. The other patterns match 27,500 files.
func benchCodeowners() string {
	var content strings.Builder
	for i := range 500 {
		switch {
		case i < 350: // An anchored pattern that matches.
			fmt.Fprintf(&content, "/dir%03d/pkg%02d/ @team\n", i%100, i%10)
		case i < 450: // An anchored pattern that matches no file.
			fmt.Fprintf(&content, "/dir%03d/missing%02d/ @team\n", i%100, i%10)
		case i < 475: // A pattern that is not anchored and matches.
			fmt.Fprintf(&content, "file%02d.go @team\n", i%50)
		default: // A pattern that is not anchored and matches no file.
			fmt.Fprintf(&content, "*.txt%02d @team\n", i%50)
		}
	}

	return content.String()
}
