// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Tests for MergeBaseFiles against real git repositories
// built with go-git.

package gitdiff

import (
	"errors"
	"fmt"
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
	root, _, baseName := newTestRepo(t)

	clonePath := t.TempDir()
	_, err := git.PlainClone(clonePath, false, &git.CloneOptions{
		URL:           root,
		Depth:         1,
		ReferenceName: plumbing.NewBranchReferenceName("feature"),
		SingleBranch:  true,
	})
	assert.NilError(t, err)

	_, err = MergeBaseFiles(t.Context(), clonePath, baseName, []string{"schema.graphql"})
	assert.Assert(t, errors.Is(err, ErrShallowClone))
}

// TestMergeBaseFilesFileBecomesDirectory checks that a path below a file of the merge-base is a path
// that did not exist, like any other new path, and is not a missing object.
func TestMergeBaseFilesFileBecomesDirectory(t *testing.T) {
	root, repo, baseName := newTestRepo(t)

	wt, err := repo.Worktree()
	assert.NilError(t, err)
	_, err = wt.Remove("schema.graphql")
	assert.NilError(t, err)
	commitFile(t, repo, root, "schema.graphql/x.graphql", "type Bar { a: String }\n")

	content, err := MergeBaseFiles(t.Context(), root, baseName, []string{"schema.graphql/x.graphql"})
	assert.NilError(t, err)
	assert.Equal(t, len(content), 0)
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
// example "blob:none"), checks out the "feature" branch of newTestRepo, and returns the clone. The
// remote of the clone is origin, so the base branch is "origin/<name>".
func newPartialClone(t *testing.T, source, filter string) string {
	t.Helper()

	runGit(t, source, "config", "uploadpack.allowFilter", "true")

	clone := filepath.Join(t.TempDir(), "clone")
	runGit(t, filepath.Dir(clone), "clone", "-q", "--filter="+filter, "--no-checkout", "file://"+source, clone)
	runGit(t, clone, "checkout", "-q", "feature")

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
	clone := newPartialClone(t, root, "blob:none")

	// Without the fetch on demand, the blob is not in the clone.
	// rev-list prints a missing object as "?<hash>" and does not fetch it.
	objects, err := gitOutput(t, clone, "rev-list", "--objects", "--missing=print", "HEAD")
	assert.NilError(t, err)
	assert.Assert(t, strings.Contains(objects, "?"+blobHash("type Foo { a: String }\n")), "the blob is already in the clone")

	content, err := MergeBaseFiles(t.Context(), clone, "origin/"+baseName, []string{"schema.graphql"})
	assert.NilError(t, err)
	assert.Equal(t, content["schema.graphql"], "type Foo { a: String }\n")
}

// TestMergeBaseFilesTreelessClone checks that a missing tree is an error with the same advice as
// a missing blob. go-git cannot read it, and git is only used for blobs.
func TestMergeBaseFilesTreelessClone(t *testing.T) {
	root, _, baseName := newTestRepo(t)
	clone := newPartialClone(t, root, "tree:0")

	_, err := MergeBaseFiles(t.Context(), clone, "origin/"+baseName, []string{"schema.graphql"})
	assert.Assert(t, errors.Is(err, ErrMissingObject), "got %v", err)

	// With no file to read, the tree is not needed.
	content, err := MergeBaseFiles(t.Context(), clone, "origin/"+baseName, nil)
	assert.NilError(t, err)
	assert.Equal(t, len(content), 0)
}

// gitOutput runs git in dir and returns its trimmed standard output. Unlike runGit, it uses the git
// configuration of the user and the system. The error of a failed command includes its standard error.
func gitOutput(t *testing.T, dir string, args ...string) (string, error) {
	t.Helper()

	cmd := exec.CommandContext(t.Context(), "git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := errors.AsType[*exec.ExitError](err); ok {
			err = fmt.Errorf("%w: %s", err, strings.TrimSpace(string(exitErr.Stderr)))
		}

		return "", fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}

	return strings.TrimSpace(string(out)), nil
}

// skipOrFailInCI skips the test, or fails it in CircleCI. The test that uses it checks the real
// checkout of CircleCI, so a skip there would hide a broken fallback for good.
func skipOrFailInCI(t *testing.T, format string, args ...any) {
	t.Helper()

	if os.Getenv("CIRCLECI") == "true" {
		t.Fatalf(format, args...)
	}
	t.Skipf(format, args...)
}

// remoteKind names the transport of a git remote URL, without the host or any credentials.
func remoteKind(url string) string {
	switch {
	case strings.HasPrefix(url, "https://"):
		return "https"
	case strings.HasPrefix(url, "git@"), strings.HasPrefix(url, "ssh://"):
		return "ssh"
	default:
		return "another transport"
	}
}

// missingBlobOfCheckout returns the hash of a blob that is missing from the repository under test.
// It skips the test, or fails it in CircleCI, unless that repository is a blobless clone, which is
// the default checkout of CircleCI.
func missingBlobOfCheckout(t *testing.T) plumbing.Hash {
	t.Helper()

	filter, err := gitOutput(t, ".", "config", "--get", "remote.origin.partialclonefilter")
	if filter != "blob:none" {
		skipOrFailInCI(t, "the repository is not a blobless clone: filter %q, error: %v", filter, err)
	}

	// rev-list prints a missing object as "?<hash>" and does not fetch it.
	objects, err := gitOutput(t, ".", "rev-list", "--objects", "--missing=print", "--max-count=20", "HEAD")
	assert.NilError(t, err)

	for line := range strings.SplitSeq(objects, "\n") {
		if hash, ok := strings.CutPrefix(line, "?"); ok {
			return plumbing.NewHash(hash)
		}
	}
	skipOrFailInCI(t, "the latest commits of the clone have no missing blob")

	return plumbing.ZeroHash
}

// TestBlobViaGitFetchesFromTheRemote reads a blob that is missing from the repository under test,
// so git must fetch it from the real remote with the credentials of the checkout. In a blobless
// clone of a developer, it fetches from the network.
func TestBlobViaGitFetchesFromTheRemote(t *testing.T) {
	missing := missingBlobOfCheckout(t)

	content, err := blobViaGit(t.Context(), ".", missing)
	url, _ := gitOutput(t, ".", "config", "--get", "remote.origin.url")
	assert.NilError(t, err, "git could not fetch the blob from a remote that uses %s", remoteKind(url))
	assert.Equal(t, blobHash(content), missing.String())
}

// TestBlobViaGoGitFetchesFromTheRemote fetches a blob that is missing from the repository under
// test with go-git, from the real remote URL of the checkout and with the authentication
// candidates of authCandidates. In CircleCI, go-git may not reach the checkout key, because it
// reads no IdentityFile from the ssh configuration. A failure there is information about the
// go-git fetch, not a bug of MergeBaseFiles: the git fallback still reads the blob.
func TestBlobViaGoGitFetchesFromTheRemote(t *testing.T) {
	missing := missingBlobOfCheckout(t)

	repo, err := git.PlainOpenWithOptions(".", &git.PlainOpenOptions{DetectDotGit: true})
	assert.NilError(t, err)
	remote := originGoGitRemote(repo)
	if remote == nil {
		skipOrFailInCI(t, "go-git does not fetch from the origin remote, which is not an ssh, http or https URL")
	}

	// The error names the kinds of authentication that go-git tried, and no secrets.
	content, err := remote.fetch(t.Context(), missing)
	assert.NilError(t, err, "go-git could not fetch the blob from a remote that uses %s", remoteKind(remote.url))
	assert.Equal(t, blobHash(content), missing.String())
}
