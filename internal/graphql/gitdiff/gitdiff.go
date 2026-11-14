// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Reads the merge-base version of files from a git
// repository's object store, for devbase graphql lint --diff and
// devbase lint codeowners --owners-if-changed.

// Package gitdiff resolves the merge-base commit between HEAD and a
// base ref using go-git, then reads file content from that commit's
// tree. It has no GraphQL-specific knowledge; the graphql lint --diff
// subcommand combines its output with the working tree's own content
// to compute the violations that are new. The lint codeowners
// subcommand uses it to find out if CODEOWNERS changed.
package gitdiff

import (
	"errors"
	"fmt"
	"path/filepath"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/storer"
)

// ErrShallowClone is returned by MergeBaseFiles when the repository is
// a shallow clone. A shallow clone's object store can be missing the
// merge-base commit, so --diff mode fails fast rather than risk an
// incomplete or wrong comparison. Deepen the clone (for example, `git
// fetch --unshallow origin <branch> <base>`) and retry.
var ErrShallowClone = errors.New("shallow clone: deepen it (e.g. `git fetch --unshallow`) before using --diff")

// errNoCommonAncestor is returned by resolveMergeBase when HEAD and
// base share no history, which `git merge-base` also rejects.
var errNoCommonAncestor = errors.New("no common ancestor")

// MergeBaseFiles opens the git repository containing dir, resolves the
// merge-base commit between HEAD and base, and reads each of files
// (paths relative to dir) from that commit's tree. The returned map is
// keyed by the same strings given in files; a file that did not yet
// exist at the merge-base is omitted rather than treated as an error,
// since it can't have had any violations at that point.
func MergeBaseFiles(dir, base string, files []string) (map[string]string, error) {
	repo, _, mergeBase, err := openMergeBase(dir, base)
	if err != nil {
		return nil, err
	}

	return readFiles(repo, mergeBase, "merge-base", dir, files)
}

// MergeBaseOrParentFiles is like MergeBaseFiles, but when the merge base
// is HEAD itself, as on the base branch, it reads the files from the
// first parent of HEAD, and firstParent is true. When HEAD has no
// parent, no file existed before it, so the map is empty. When the
// first parent of HEAD cannot be read, the error comes with
// firstParent true.
func MergeBaseOrParentFiles(dir, base string, files []string) (contents map[string]string, firstParent bool, err error) {
	repo, head, mergeBase, err := openMergeBase(dir, base)
	if err != nil {
		return nil, false, err
	}

	if mergeBase.Hash != head.Hash {
		contents, err = readFiles(repo, mergeBase, "merge-base", dir, files)
		return contents, false, err
	}

	if head.NumParents() == 0 {
		return map[string]string{}, true, nil
	}

	parent, err := head.Parent(0)
	if err != nil {
		return nil, true, fmt.Errorf("load the first parent of HEAD: %w", err)
	}

	contents, err = readFiles(repo, parent, "the first parent of HEAD", dir, files)
	return contents, true, err
}

// openMergeBase opens the git repository containing dir, and returns it
// with the HEAD commit and the merge-base commit of HEAD and base.
func openMergeBase(dir, base string) (repo *git.Repository, head, mergeBase *object.Commit, err error) {
	// The common git directory holds the refs, objects and shallow file of a linked worktree.
	repo, err = git.PlainOpenWithOptions(dir, &git.PlainOpenOptions{DetectDotGit: true, EnableDotGitCommonDir: true})
	if err != nil {
		return nil, nil, nil, fmt.Errorf("open git repository containing %s: %w", dir, err)
	}

	if err := checkNotShallow(repo); err != nil {
		return nil, nil, nil, err
	}

	head, mergeBase, err = resolveMergeBase(repo, base)
	if err != nil {
		return nil, nil, nil, err
	}

	return repo, head, mergeBase, nil
}

// checkNotShallow returns ErrShallowClone if repo has any shallow
// grafts recorded, i.e. it was cloned or fetched with a depth limit.
func checkNotShallow(repo *git.Repository) error {
	shallowStorer, ok := repo.Storer.(storer.ShallowStorer)
	if !ok {
		return nil
	}
	hashes, err := shallowStorer.Shallow()
	if err != nil {
		return fmt.Errorf("check for shallow clone: %w", err)
	}
	if len(hashes) > 0 {
		return ErrShallowClone
	}
	return nil
}

// resolveMergeBase resolves HEAD and base to commits and returns the
// HEAD commit and their merge-base, the best common ancestor, mirroring
// `git merge-base`.
func resolveMergeBase(repo *git.Repository, base string) (head, mergeBase *object.Commit, err error) {
	headRef, err := repo.Head()
	if err != nil {
		return nil, nil, fmt.Errorf("resolve HEAD: %w", err)
	}
	head, err = repo.CommitObject(headRef.Hash())
	if err != nil {
		return nil, nil, fmt.Errorf("load HEAD commit: %w", err)
	}

	baseHash, err := repo.ResolveRevision(plumbing.Revision(base))
	if err != nil {
		return nil, nil, fmt.Errorf("resolve --base %q: %w", base, err)
	}
	baseCommit, err := repo.CommitObject(*baseHash)
	if err != nil {
		return nil, nil, fmt.Errorf("load --base %q commit: %w", base, err)
	}

	bases, err := head.MergeBase(baseCommit)
	if err != nil {
		return nil, nil, fmt.Errorf("compute merge-base of HEAD and %q: %w", base, err)
	}
	if len(bases) == 0 {
		return nil, nil, fmt.Errorf("%w: HEAD and %q", errNoCommonAncestor, base)
	}
	return head, bases[0], nil
}

// repoRoot returns the absolute path of repo's working tree root, the
// directory git trees are rooted at regardless of the caller's dir.
func repoRoot(repo *git.Repository) (string, error) {
	wt, err := repo.Worktree()
	if err != nil {
		return "", fmt.Errorf("get worktree: %w", err)
	}
	return wt.Filesystem.Root(), nil
}

// readFiles reads each of files (given relative to dir) from commit's
// tree, returning a map keyed by the original file string. name
// describes commit in errors.
func readFiles(repo *git.Repository, commit *object.Commit, name, dir string, files []string) (map[string]string, error) {
	root, err := repoRoot(repo)
	if err != nil {
		return nil, err
	}

	// root, from go-git's Worktree, and dir/files, from the caller, can
	// reach the same directory through different symlinks -- e.g. on
	// macOS, where a temp directory's real path is under /private but
	// the caller's os.Getwd() reports the /var symlink. filepath.Rel on
	// two such paths silently computes the wrong relative path, so
	// resolve both sides before comparing them.
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return nil, fmt.Errorf("resolve symlinks for repository root %s: %w", root, err)
	}

	contents := make(map[string]string, len(files))
	for _, file := range files {
		absPath := file
		if !filepath.IsAbs(absPath) {
			absPath = filepath.Join(dir, file)
		}
		realPath, err := filepath.EvalSymlinks(absPath)
		if err != nil {
			return nil, fmt.Errorf("resolve symlinks for %s: %w", file, err)
		}
		relPath, err := filepath.Rel(realRoot, realPath)
		if err != nil {
			return nil, fmt.Errorf("resolve %s relative to repository root %s: %w", file, root, err)
		}

		f, err := commit.File(relPath)
		if errors.Is(err, object.ErrFileNotFound) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("read %s at %s: %w", file, name, err)
		}

		content, err := f.Contents()
		if err != nil {
			return nil, fmt.Errorf("read %s at %s: %w", file, name, err)
		}
		contents[file] = content
	}
	return contents, nil
}
