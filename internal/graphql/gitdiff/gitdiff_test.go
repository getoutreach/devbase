// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Tests for MergeBaseFiles against real git repositories
// built with go-git.

package gitdiff

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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
		content, err := MergeBaseFiles(t.Context(), root, baseName, []string{"schema.graphql"})
		assert.NilError(t, err)
		assert.Equal(t, content["schema.graphql"], "type Foo { a: String }\n")
	})

	t.Run("omits a file that did not exist at the merge-base", func(t *testing.T) {
		commitFile(t, repo, root, "new.graphql", "type Bar { a: String }\n")

		content, err := MergeBaseFiles(t.Context(), root, baseName, []string{"schema.graphql", "new.graphql"})
		assert.NilError(t, err)
		_, ok := content["new.graphql"]
		assert.Equal(t, ok, false)
		assert.Equal(t, len(content), 1)
	})

	t.Run("resolves a file path relative to a subdirectory of the repository", func(t *testing.T) {
		commitFile(t, repo, root, "sub/schema.graphql", "type Baz { a: String b: String }\n")

		content, err := MergeBaseFiles(t.Context(), filepath.Join(root, "sub"), baseName, []string{"schema.graphql"})
		assert.NilError(t, err)
		assert.Equal(t, content["schema.graphql"], "type Baz { a: String }\n")
	})
}

// runGit runs git in dir, without the git configuration of the user or the system. It fails the
// test when git fails.
func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()

	cmd := exec.CommandContext(t.Context(), "git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_CONFIG_NOSYSTEM=1")
	out, err := cmd.CombinedOutput()
	assert.NilError(t, err, "git %s: %s", strings.Join(args, " "), out)
}

// newPartialClone clones the repository at source without the objects that filter excludes (for
// example "blob:none"), checks out branch, and returns the clone. The remote of the clone is
// origin, so the base branch is "origin/<name>".
func newPartialClone(t *testing.T, source, filter, branch string) string {
	t.Helper()

	runGit(t, source, "config", "uploadpack.allowFilter", "true")

	clone := filepath.Join(t.TempDir(), "clone")
	runGit(t, filepath.Dir(clone), "clone", "-q", "--filter="+filter, "--no-checkout", "file://"+source, clone)
	runGit(t, clone, "checkout", "-q", branch)

	return clone
}

// blobHash returns the hash of a blob with the given content.
func blobHash(content string) string {
	return plumbing.ComputeHash(plumbing.BlobObject, []byte(content)).String()
}

// removeBlob deletes the loose object of the blob with the given content from the repository at
// root.
func removeBlob(t *testing.T, root, content string) {
	t.Helper()

	hash := blobHash(content)
	assert.NilError(t, os.Remove(filepath.Join(root, ".git", "objects", hash[:2], hash[2:])))
}

// TestMergeBaseFilesMissingObject checks that a file whose blob is missing from the object store,
// and that git cannot fetch either (there is no remote), is an error and is not mistaken for a file
// that is new.
func TestMergeBaseFilesMissingObject(t *testing.T) {
	root, _, baseName := newTestRepo(t)
	removeBlob(t, root, "type Foo { a: String }\n")
	removeBlob(t, root, "type Baz { a: String }\n")

	for _, file := range []string{"schema.graphql", "sub/schema.graphql"} {
		t.Run(file, func(t *testing.T) {
			_, err := MergeBaseFiles(t.Context(), root, baseName, []string{file})
			assert.Assert(t, errors.Is(err, ErrMissingObject), "got %v", err)
			assert.ErrorContains(t, err, file)
		})
	}
}

// TestMergeBaseFilesMissingSubtree checks that a subtree that is missing from the object store is
// an error, and is not mistaken for a path that does not exist.
func TestMergeBaseFilesMissingSubtree(t *testing.T) {
	root, repo, baseName := newTestRepo(t)

	baseRef, err := repo.Reference(plumbing.NewBranchReferenceName(baseName), true)
	assert.NilError(t, err)
	baseCommit, err := repo.CommitObject(baseRef.Hash())
	assert.NilError(t, err)
	baseTree, err := baseCommit.Tree()
	assert.NilError(t, err)
	sub, err := baseTree.FindEntry("sub")
	assert.NilError(t, err)
	hash := sub.Hash.String()
	assert.NilError(t, os.Remove(filepath.Join(root, ".git", "objects", hash[:2], hash[2:])))

	_, err = MergeBaseFiles(t.Context(), root, baseName, []string{"sub/schema.graphql"})
	assert.Assert(t, errors.Is(err, ErrMissingObject), "got %v", err)
}

// TestMergeBaseFilesDirectoryIsOmitted checks that a directory is treated like a file that is new.
func TestMergeBaseFilesDirectoryIsOmitted(t *testing.T) {
	root, _, baseName := newTestRepo(t)

	content, err := MergeBaseFiles(t.Context(), root, baseName, []string{"sub"})
	assert.NilError(t, err)
	assert.Equal(t, len(content), 0)
}

// TestMergeBaseFilesBloblessClone checks that a blob that a blobless clone does not have is read
// through git, which fetches it from the remote.
func TestMergeBaseFilesBloblessClone(t *testing.T) {
	root, _, baseName := newTestRepo(t)
	clone := newPartialClone(t, root, "blob:none", "feature")

	// Without the fetch on demand, the blob is not in the clone.
	check := exec.CommandContext(t.Context(), "git", "cat-file", "-e", blobHash("type Foo { a: String }\n"))
	check.Dir = clone
	check.Env = append(os.Environ(), "GIT_NO_LAZY_FETCH=1")
	assert.Assert(t, check.Run() != nil, "the blob is already in the clone")

	content, err := MergeBaseFiles(t.Context(), clone, "origin/"+baseName, []string{"schema.graphql"})
	assert.NilError(t, err)
	assert.Equal(t, content["schema.graphql"], "type Foo { a: String }\n")
}

// TestMergeBaseFilesTreelessClone checks that a missing tree is an error with the same advice as
// a missing blob. go-git cannot read it, and git is only used for blobs.
func TestMergeBaseFilesTreelessClone(t *testing.T) {
	root, _, baseName := newTestRepo(t)
	clone := newPartialClone(t, root, "tree:0", "feature")

	_, err := MergeBaseFiles(t.Context(), clone, "origin/"+baseName, []string{"schema.graphql"})
	assert.Assert(t, errors.Is(err, ErrMissingObject), "got %v", err)
}

// TestMergeBaseFilesThroughSymlinkedDir guards against a mismatch
// between go-git's resolved worktree root and the caller's own path,
// as happens on macOS: a temp directory's real path is under
// /private, but os.Getwd() reports the /var symlink to it. Passing
// dir through a symlink here reproduces that mismatch on any
// platform.
func TestMergeBaseFilesThroughSymlinkedDir(t *testing.T) {
	root, _, baseName := newTestRepo(t)

	symlinked := root + "-symlink"
	assert.NilError(t, os.Symlink(root, symlinked))
	t.Cleanup(func() { assert.NilError(t, os.Remove(symlinked)) })

	content, err := MergeBaseFiles(t.Context(), symlinked, baseName, []string{"schema.graphql"})
	assert.NilError(t, err)
	assert.Equal(t, content["schema.graphql"], "type Foo { a: String }\n")
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

	_, err = MergeBaseFiles(t.Context(), clonePath, "master", []string{"schema.graphql"})
	assert.Assert(t, errors.Is(err, ErrShallowClone))
}
