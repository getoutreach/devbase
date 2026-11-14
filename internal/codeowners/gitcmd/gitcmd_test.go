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

	var gitError *gitcmd.Error
	assert.Assert(t, errors.As(err, &gitError))
	assert.Equal(t, gitError.Dir, dir)
	assert.Assert(t, strings.HasPrefix(gitError.Stderr, "fatal: "), gitError.Stderr)

	var exitError *exec.ExitError
	assert.Assert(t, errors.As(err, &exitError))
}

// TestRunErrorWithoutStderr checks the error of a git command that fails without output.
func TestRunErrorWithoutStderr(t *testing.T) {
	dir := newRepo(t)

	_, err := gitcmd.Run(t.Context(), dir, false, "rev-parse", "--verify", "--quiet", "missing")
	assert.Error(t, err, "git rev-parse --verify --quiet missing in "+dir+": exit status 1")
}

func TestListIndex(t *testing.T) {
	dir := newRepo(t)
	run := func(args ...string) {
		t.Helper()
		_, err := gitcmd.Run(t.Context(), dir, false, args...)
		assert.NilError(t, err)
	}
	assert.NilError(t, os.WriteFile(filepath.Join(dir, "run.sh"), nil, 0o700))
	run("add", "--chmod=+x", "run.sh")
	run("update-index", "--add", "--cacheinfo", "160000,e69de29bb2d1d6434b8b29ae775ad8c2e48c5391,sub module")
	run("update-index", "--add", "--cacheinfo", "120000,e69de29bb2d1d6434b8b29ae775ad8c2e48c5391,link")

	entries, err := gitcmd.ListIndex(t.Context(), dir, false)
	assert.NilError(t, err)
	assert.DeepEqual(t, entries, []gitcmd.IndexEntry{
		{Mode: "100644", Path: "a.txt"},
		{Mode: "120000", Path: "link"},
		{Mode: "100755", Path: "run.sh"},
		{Mode: "160000", Path: "sub module"},
	})
	kinds := func(e gitcmd.IndexEntry) [3]bool {
		return [3]bool{e.IsRegularFile(), e.IsSymbolicLink(), e.IsSubmodule()}
	}
	assert.Equal(t, kinds(entries[0]), [3]bool{true, false, false})
	assert.Equal(t, kinds(entries[1]), [3]bool{false, true, false})
	assert.Equal(t, kinds(entries[2]), [3]bool{true, false, false})
	assert.Equal(t, kinds(entries[3]), [3]bool{false, false, true})

	entries, err = gitcmd.ListIndex(t.Context(), dir, false, "--", "missing")
	assert.NilError(t, err)
	assert.Equal(t, len(entries), 0)

	_, err = gitcmd.ListIndex(t.Context(), t.TempDir(), false)
	assert.ErrorContains(t, err, "not a git repository")
}
