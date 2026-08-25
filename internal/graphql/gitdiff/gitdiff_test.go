// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Tests for MergeBaseFiles against real git repositories
// built with go-git.

package gitdiff

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"gotest.tools/v3/assert"
)

var testSignature = &object.Signature{Name: "test", Email: "test@example.com", When: time.Unix(0, 0)}

// commitFile writes path (relative to repo's root) with content, stages
// it, and commits it to repo's current branch.
func commitFile(t *testing.T, repo *git.Repository, root, path, content string) {
	t.Helper()
	full := filepath.Join(root, path)
	assert.NilError(t, os.MkdirAll(filepath.Dir(full), 0o755))
	assert.NilError(t, os.WriteFile(full, []byte(content), 0o600))

	wt, err := repo.Worktree()
	assert.NilError(t, err)
	_, err = wt.Add(path)
	assert.NilError(t, err)
	_, err = wt.Commit("commit "+path, &git.CommitOptions{Author: testSignature})
	assert.NilError(t, err)
}

// newTestRepo creates a git repository at root with "schema.graphql"
// and "sub/schema.graphql" committed to the default branch (baseName),
// then checks out a "feature" branch one commit ahead that modifies
// "schema.graphql". HEAD ends on "feature", the same shape as a PR
// branch checked out in CI, with baseName reachable as the merge-base
// target.
func newTestRepo(t *testing.T) (root string, repo *git.Repository, baseName string) {
	t.Helper()
	root = t.TempDir()

	var err error
	repo, err = git.PlainInit(root, false)
	assert.NilError(t, err)

	commitFile(t, repo, root, "schema.graphql", "type Foo { a: String }\n")
	commitFile(t, repo, root, "sub/schema.graphql", "type Baz { a: String }\n")

	head, err := repo.Head()
	assert.NilError(t, err)
	baseName = head.Name().Short()

	wt, err := repo.Worktree()
	assert.NilError(t, err)
	assert.NilError(t, wt.Checkout(&git.CheckoutOptions{
		Branch: plumbing.NewBranchReferenceName("feature"),
		Create: true,
	}))
	commitFile(t, repo, root, "schema.graphql", "type Foo { a: String b: String }\n")

	return root, repo, baseName
}

func TestMergeBaseFiles(t *testing.T) {
	root, repo, baseName := newTestRepo(t)

	t.Run("returns the merge-base content of an existing file", func(t *testing.T) {
		content, err := MergeBaseFiles(root, baseName, []string{"schema.graphql"})
		assert.NilError(t, err)
		assert.Equal(t, content["schema.graphql"], "type Foo { a: String }\n")
	})

	t.Run("omits a file that did not exist at the merge-base", func(t *testing.T) {
		commitFile(t, repo, root, "new.graphql", "type Bar { a: String }\n")

		content, err := MergeBaseFiles(root, baseName, []string{"schema.graphql", "new.graphql"})
		assert.NilError(t, err)
		_, ok := content["new.graphql"]
		assert.Equal(t, ok, false)
		assert.Equal(t, len(content), 1)
	})

	t.Run("resolves a file path relative to a subdirectory of the repository", func(t *testing.T) {
		commitFile(t, repo, root, "sub/schema.graphql", "type Baz { a: String b: String }\n")

		content, err := MergeBaseFiles(filepath.Join(root, "sub"), baseName, []string{"schema.graphql"})
		assert.NilError(t, err)
		assert.Equal(t, content["schema.graphql"], "type Baz { a: String }\n")
	})
}

func TestMergeBaseFilesShallowClone(t *testing.T) {
	root, _, _ := newTestRepo(t)

	clonePath := t.TempDir()
	_, err := git.PlainClone(clonePath, false, &git.CloneOptions{
		URL:           root,
		Depth:         1,
		ReferenceName: plumbing.NewBranchReferenceName("feature"),
		SingleBranch:  true,
	})
	assert.NilError(t, err)

	_, err = MergeBaseFiles(clonePath, "master", []string{"schema.graphql"})
	assert.Assert(t, errors.Is(err, ErrShallowClone))
}
