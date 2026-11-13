// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Tests for loading the CODEOWNERS file from the git index.

package codeowners_test

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/getoutreach/devbase/v2/internal/codeowners"
	"github.com/getoutreach/devbase/v2/internal/gittest"
	"gotest.tools/v3/assert"
)

// TestMain isolates the git commands of the tests from the git environment of the caller.
func TestMain(m *testing.M) {
	os.Exit(gittest.Run(m))
}

// runGit runs git with args in dir and fails the test on error.
func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()

	cmd := exec.CommandContext(t.Context(), "git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	assert.NilError(t, err, "git %v: %s", args, out)
}

// newStagedRepo creates a git repository in a temporary directory with a file for each path and
// content in files, all staged. It returns the directory.
func newStagedRepo(t *testing.T, files map[string]string) string {
	t.Helper()

	repo := t.TempDir()
	runGit(t, repo, "init", "-q")
	for rel, content := range files {
		writeFile(t, repo, rel, content)
	}
	runGit(t, repo, "add", "-A")

	return repo
}

func TestLoadFromIndexIgnoresUnstagedChanges(t *testing.T) {
	repo := newStagedRepo(t, map[string]string{".github/CODEOWNERS": "* @staged\n"})
	writeFile(t, repo, ".github/CODEOWNERS", "* @unstaged\n")

	entries, err := codeowners.LoadFromIndex(t.Context(), repo, false)
	assert.NilError(t, err)
	assert.DeepEqual(t, entries, []codeowners.Entry{{LineNo: 1, Pattern: "*", Owners: []string{"@staged"}}})
}

func TestLoadFromIndexLocations(t *testing.T) {
	for _, rel := range []string{"CODEOWNERS", "docs/CODEOWNERS", ".github/CODEOWNERS"} {
		t.Run(rel, func(t *testing.T) {
			repo := newStagedRepo(t, map[string]string{rel: "* @a\n", "docs/CODEOWNERS.md": "* @b\n"})

			entries, err := codeowners.LoadFromIndex(t.Context(), repo, false)
			assert.NilError(t, err)
			assert.DeepEqual(t, entries, []codeowners.Entry{{LineNo: 1, Pattern: "*", Owners: []string{"@a"}}})
		})
	}

	t.Run("a directory named CODEOWNERS", func(t *testing.T) {
		repo := newStagedRepo(t, map[string]string{"CODEOWNERS": "* @a\n", "docs/CODEOWNERS/x": "* @b\n"})

		entries, err := codeowners.LoadFromIndex(t.Context(), repo, false)
		assert.NilError(t, err)
		assert.DeepEqual(t, entries, []codeowners.Entry{{LineNo: 1, Pattern: "*", Owners: []string{"@a"}}})
	})
}

func TestLoadFromIndexSubdirectory(t *testing.T) {
	repo := newStagedRepo(t, map[string]string{"CODEOWNERS": "* @root\n", "svc/CODEOWNERS": "* @svc\n"})

	entries, err := codeowners.LoadFromIndex(t.Context(), filepath.Join(repo, "svc"), false)
	assert.NilError(t, err)
	assert.DeepEqual(t, entries, []codeowners.Entry{{LineNo: 1, Pattern: "*", Owners: []string{"@svc"}}})
}

func TestLoadFromIndexFailure(t *testing.T) {
	t.Run("an unstaged file is not found", func(t *testing.T) {
		repo := newStagedRepo(t, map[string]string{"a.go": ""})
		writeFile(t, repo, "CODEOWNERS", "* @a\n")

		entries, err := codeowners.LoadFromIndex(t.Context(), repo, false)
		assert.Error(t, err, "No CODEOWNERS found in the root, docs/, or .github/ directory of the repository "+repo)
		assert.Assert(t, errors.Is(err, codeowners.ErrNotFound))
		assert.Assert(t, entries == nil)
	})

	t.Run("more than one file", func(t *testing.T) {
		repo := newStagedRepo(t, map[string]string{"CODEOWNERS": "", ".github/CODEOWNERS": ""})

		entries, err := codeowners.LoadFromIndex(t.Context(), repo, false)
		assert.Error(t, err,
			"Multiple CODEOWNERS files found in the ./CODEOWNERS and ./.github/CODEOWNERS locations of the repository "+repo)
		assert.Assert(t, errors.Is(err, codeowners.ErrMultipleFiles))
		assert.Assert(t, entries == nil)
	})

	t.Run("not a git repository", func(t *testing.T) {
		repo := t.TempDir()
		writeFile(t, repo, "CODEOWNERS", "* @a\n")

		_, err := codeowners.LoadFromIndex(t.Context(), repo, false)
		assert.ErrorContains(t, err, "not a git repository")
	})
}

func TestReadFromIndex(t *testing.T) {
	repo := newStagedRepo(t, map[string]string{"docs/CODEOWNERS": "* @staged\n"})
	writeFile(t, repo, "docs/CODEOWNERS", "* @unstaged\n")

	path, content, err := codeowners.ReadFromIndex(t.Context(), repo, false)
	assert.NilError(t, err)
	assert.Equal(t, path, "docs/CODEOWNERS")
	assert.Equal(t, string(content), "* @staged\n")
}
