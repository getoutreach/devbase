// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Reads the merge-base version of files from a git
// repository's object store, for devbase graphql lint --diff.

// Package gitdiff resolves the merge-base commit between HEAD and a
// base ref using go-git, then reads file content from that commit's
// tree. It has no GraphQL-specific knowledge; the graphql lint --diff
// subcommand combines its output with the working tree's own content
// to compute the violations that are new.
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
	repo, err := git.PlainOpenWithOptions(dir, &git.PlainOpenOptions{DetectDotGit: true})
	if err != nil {
		return nil, fmt.Errorf("open git repository containing %s: %w", dir, err)
	}

	if err := checkNotShallow(repo); err != nil {
		return nil, err
	}

	mergeBase, err := resolveMergeBase(repo, base)
	if err != nil {
		return nil, err
	}

	root, err := repoRoot(repo)
	if err != nil {
		return nil, err
	}

	return readFiles(mergeBase, root, dir, files)
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

// resolveMergeBase resolves HEAD and base to commits and returns their
// merge-base, the best common ancestor, mirroring `git merge-base`.
func resolveMergeBase(repo *git.Repository, base string) (*object.Commit, error) {
	head, err := repo.Head()
	if err != nil {
		return nil, fmt.Errorf("resolve HEAD: %w", err)
	}
	headCommit, err := repo.CommitObject(head.Hash())
	if err != nil {
		return nil, fmt.Errorf("load HEAD commit: %w", err)
	}

	baseHash, err := repo.ResolveRevision(plumbing.Revision(base))
	if err != nil {
		return nil, fmt.Errorf("resolve --base %q: %w", base, err)
	}
	baseCommit, err := repo.CommitObject(*baseHash)
	if err != nil {
		return nil, fmt.Errorf("load --base %q commit: %w", base, err)
	}

	bases, err := headCommit.MergeBase(baseCommit)
	if err != nil {
		return nil, fmt.Errorf("compute merge-base of HEAD and %q: %w", base, err)
	}
	if len(bases) == 0 {
		return nil, fmt.Errorf("%w: HEAD and %q", errNoCommonAncestor, base)
	}
	return bases[0], nil
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

// readFiles reads each of files (given relative to dir) from
// mergeBase's tree, returning a map keyed by the original file string.
func readFiles(mergeBase *object.Commit, root, dir string, files []string) (map[string]string, error) {
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

		f, err := mergeBase.File(relPath)
		if errors.Is(err, object.ErrFileNotFound) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("read %s at merge-base: %w", file, err)
		}

		content, err := f.Contents()
		if err != nil {
			return nil, fmt.Errorf("read %s at merge-base: %w", file, err)
		}
		contents[file] = content
	}
	return contents, nil
}
