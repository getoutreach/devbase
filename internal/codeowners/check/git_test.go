// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Tests for the git helper.

package check

import (
	"testing"

	"gotest.tools/v3/assert"
)

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
