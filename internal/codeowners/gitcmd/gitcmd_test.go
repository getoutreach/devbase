// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Tests for running git commands.

package gitcmd_test

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/getoutreach/devbase/v2/internal/codeowners/gitcmd"
	"github.com/getoutreach/devbase/v2/internal/gittest"
	"gotest.tools/v3/assert"
)

// TestMain isolates the git commands of the tests from the git environment of the caller.
func TestMain(m *testing.M) {
	os.Exit(gittest.Run(m))
}

// newRepo creates a git repository in a temporary directory with the staged file a.txt, and
// returns the directory.
func newRepo(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	assert.NilError(t, os.WriteFile(filepath.Join(dir, "a.txt"), nil, 0o600))
	for _, args := range [][]string{{"init", "-q"}, {"add", "a.txt"}} {
		cmd := exec.CommandContext(t.Context(), "git", args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		assert.NilError(t, err, "git %v: %s", args, out)
	}

	return dir
}

func TestCommand(t *testing.T) {
	dir := "/some/repo"

	t.Run("trusted", func(t *testing.T) {
		cmd := gitcmd.Command(t.Context(), dir, true, "ls-files", "-z")
		assert.DeepEqual(t, cmd.Args, []string{"git", "-c", "safe.directory=" + dir, "ls-files", "-z"})
		assert.Equal(t, cmd.Dir, dir)
	})

	t.Run("not trusted", func(t *testing.T) {
		cmd := gitcmd.Command(t.Context(), dir, false, "ls-files", "-z")
		assert.DeepEqual(t, cmd.Args, []string{"git", "ls-files", "-z"})
		assert.Equal(t, cmd.Dir, dir)
	})
}

func TestRun(t *testing.T) {
	dir := newRepo(t)

	out, err := gitcmd.Run(t.Context(), dir, false, "ls-files")
	assert.NilError(t, err)
	assert.Equal(t, string(out), "a.txt\n")
}

func TestRunError(t *testing.T) {
	dir := newRepo(t)

	_, err := gitcmd.Run(t.Context(), dir, false, "cat-file", "blob", ":missing.txt")
	assert.ErrorContains(t, err, "git cat-file blob :missing.txt in "+dir+": exit status 128: fatal: ")
	assert.ErrorContains(t, err, "missing.txt")

	var gitErr *gitcmd.Error
	assert.Assert(t, errors.As(err, &gitErr))
	assert.Equal(t, gitErr.Dir, dir)
	assert.Assert(t, strings.HasPrefix(gitErr.Stderr, "fatal: "), gitErr.Stderr)

	var exitErr *exec.ExitError
	assert.Assert(t, errors.As(err, &exitErr))
}
