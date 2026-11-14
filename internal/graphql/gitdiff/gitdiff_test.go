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

	content, err := MergeBaseFiles(symlinked, baseName, []string{"schema.graphql"})
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

	_, err = MergeBaseFiles(clonePath, "master", []string{"schema.graphql"})
	assert.Assert(t, errors.Is(err, ErrShallowClone))
}

// TestMergeBaseOrParentFiles checks the commit that MergeBaseOrParentFiles reads the files from:
// the merge base of HEAD and the base, or the first parent of HEAD when HEAD is the merge base.
func TestMergeBaseOrParentFiles(t *testing.T) {
	files := []string{"schema.graphql", "sub/schema.graphql"}

	t.Run("a branch ahead of the base reads the merge base", func(t *testing.T) {
		root, _, baseName := newTestRepo(t)

		content, firstParent, err := MergeBaseOrParentFiles(root, baseName, files)
		assert.NilError(t, err)
		assert.Equal(t, firstParent, false)
		assert.DeepEqual(t, content, map[string]string{
			"schema.graphql":     "type Foo { a: String }\n",
			"sub/schema.graphql": "type Baz { a: String }\n",
		})
	})

	t.Run("the base branch reads the first parent of HEAD", func(t *testing.T) {
		root, repo, baseName := newTestRepo(t)
		wt, err := repo.Worktree()
		assert.NilError(t, err)
		assert.NilError(t, wt.Checkout(&git.CheckoutOptions{Branch: plumbing.NewBranchReferenceName(baseName)}))

		content, firstParent, err := MergeBaseOrParentFiles(root, baseName, files)
		assert.NilError(t, err)
		assert.Equal(t, firstParent, true)
		assert.DeepEqual(t, content, map[string]string{"schema.graphql": "type Foo { a: String }\n"})
	})

	t.Run("an error of the first parent of HEAD", func(t *testing.T) {
		root, repo, baseName := newTestRepo(t)
		wt, err := repo.Worktree()
		assert.NilError(t, err)
		assert.NilError(t, wt.Checkout(&git.CheckoutOptions{Branch: plumbing.NewBranchReferenceName(baseName)}))
		head, err := repo.Head()
		assert.NilError(t, err)
		commit, err := repo.CommitObject(head.Hash())
		assert.NilError(t, err)
		parent, err := commit.Parent(0)
		assert.NilError(t, err)
		tree := parent.TreeHash.String()
		assert.NilError(t, os.Remove(filepath.Join(root, ".git", "objects", tree[:2], tree[2:])))

		_, firstParent, err := MergeBaseOrParentFiles(root, baseName, files)
		assert.ErrorContains(t, err, "at the first parent of HEAD")
		assert.Equal(t, firstParent, true)
	})

	t.Run("a root commit has no files before it", func(t *testing.T) {
		root := t.TempDir()
		repo, err := git.PlainInit(root, false)
		assert.NilError(t, err)
		commitFile(t, repo, root, "schema.graphql", "type Foo { a: String }\n")
		head, err := repo.Head()
		assert.NilError(t, err)

		content, firstParent, err := MergeBaseOrParentFiles(root, head.Name().Short(), []string{"schema.graphql"})
		assert.NilError(t, err)
		assert.Equal(t, firstParent, true)
		assert.DeepEqual(t, content, map[string]string{})
	})
}

// runGit runs git in dir and fails the test when git fails.
func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()

	gitOutput(t, dir, args...)
}

// gitOutput runs git in dir, fails the test when git fails, and returns the standard output.
func gitOutput(t *testing.T, dir string, args ...string) string {
	t.Helper()

	identity := []string{"-c", "user.name=test", "-c", "user.email=test@example.com"}
	cmd := exec.CommandContext(t.Context(), "git", append(identity, args...)...)
	cmd.Dir = dir
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	assert.NilError(t, err, "git %s: %s", strings.Join(args, " "), stderr.String())

	return string(out)
}

// TestMergeBaseFilesLinkedWorktree checks that the merge base is read when dir is a linked git
// worktree, where the refs and objects are in the common git directory.
func TestMergeBaseFilesLinkedWorktree(t *testing.T) {
	root, _, baseName := newTestRepo(t)

	worktree := filepath.Join(t.TempDir(), "linked")
	runGit(t, root, "worktree", "add", "-q", "-b", "linked", worktree, "feature")
	assert.NilError(t, os.WriteFile(filepath.Join(worktree, "new.graphql"), []byte("type Bar { a: String }\n"), 0o600))
	runGit(t, worktree, "add", "-A")
	runGit(t, worktree, "commit", "-q", "-m", "add new.graphql")

	content, err := MergeBaseFiles(worktree, baseName, []string{"schema.graphql", "new.graphql"})
	assert.NilError(t, err)
	assert.DeepEqual(t, content, map[string]string{"schema.graphql": "type Foo { a: String }\n"})
}

// TestMergeBaseFilesShallowCloneLinkedWorktree checks that a linked worktree of a shallow clone
// is detected as shallow, although the shallow file is in the common git directory.
func TestMergeBaseFilesShallowCloneLinkedWorktree(t *testing.T) {
	root, _, baseName := newTestRepo(t)

	clonePath := filepath.Join(t.TempDir(), "clone")
	runGit(t, root, "clone", "-q", "--depth", "1", "file://"+root, clonePath)
	worktree := filepath.Join(t.TempDir(), "linked")
	runGit(t, clonePath, "worktree", "add", "-q", "--detach", worktree)

	_, err := MergeBaseFiles(worktree, baseName, []string{"schema.graphql"})
	assert.Assert(t, errors.Is(err, ErrShallowClone), "%v", err)
}

// TestMergeBaseOrParentFilesBloblessClone checks that a file whose blob is missing in a blobless
// clone is left out, as if it did not exist in the compared commit.
func TestMergeBaseOrParentFilesBloblessClone(t *testing.T) {
	// The source has two commits on main, and the second commit changes .github/CODEOWNERS.
	src := t.TempDir()
	runGit(t, src, "init", "-q", "-b", "main")
	runGit(t, src, "config", "uploadpack.allowFilter", "true")
	path := filepath.Join(src, ".github", "CODEOWNERS")
	assert.NilError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	for _, content := range []string{"* @bob\n", "* @alice\n"} {
		assert.NilError(t, os.WriteFile(path, []byte(content), 0o600))
		runGit(t, src, "add", "-A")
		runGit(t, src, "commit", "-q", "-m", "change CODEOWNERS")
	}

	// The clone has no blob, and no checkout. The command reads CODEOWNERS from the working tree.
	clone := filepath.Join(t.TempDir(), "clone")
	runGit(t, src, "clone", "-q", "--filter=blob:none", "--no-checkout", "file://"+src, clone)
	blob := strings.TrimSpace(gitOutput(t, src, "rev-parse", "main~1:.github/CODEOWNERS"))
	missing := gitOutput(t, clone, "rev-list", "--objects", "--all", "--missing=print")
	assert.Assert(t, strings.Contains(missing, "?"+blob+"\n"), "the blob %s is not missing:\n%s", blob, missing)
	assert.NilError(t, os.MkdirAll(filepath.Join(clone, ".github"), 0o755))
	assert.NilError(t, os.WriteFile(filepath.Join(clone, ".github", "CODEOWNERS"), []byte("* @alice\n"), 0o600))

	content, firstParent, err := MergeBaseOrParentFiles(clone, "main", []string{".github/CODEOWNERS"})

	assert.NilError(t, err)
	assert.Equal(t, firstParent, true)
	assert.DeepEqual(t, content, map[string]string{})
}
