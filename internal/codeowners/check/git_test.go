// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Tests for the git helper.

package check

import (
	"slices"
	"testing"

	"gotest.tools/v3/assert"
)

func TestGitArgsTrustRepoDir(t *testing.T) {
	dir := "/some/repo"

	t.Run("trusted", func(t *testing.T) {
		args := gitArgs(Input{RepoDir: dir, TrustRepoDir: true}, "--cached")
		assert.DeepEqual(t, args, []string{"-c", "safe.directory=" + dir, "ls-files", "-z", "--cached"})
	})

	t.Run("not trusted", func(t *testing.T) {
		args := gitArgs(Input{RepoDir: dir}, "--cached")
		assert.Assert(t, !slices.Contains(args, "-c"))
		assert.DeepEqual(t, args, []string{"ls-files", "-z", "--cached"})
	})
}

func TestGitLsFiles(t *testing.T) {
	dir := newGitRepo(t, "a.txt", "sub/b.txt")

	files, err := gitLsFiles(t.Context(), Input{RepoDir: dir}, "--cached")
	assert.NilError(t, err)
	assert.DeepEqual(t, files, []string{"a.txt", "sub/b.txt"})
}

func TestGitLsFilesNotARepo(t *testing.T) {
	_, err := gitLsFiles(t.Context(), Input{RepoDir: t.TempDir()}, "--cached")
	assert.ErrorContains(t, err, "not a git repository")
}
