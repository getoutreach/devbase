// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: The release dry run of a pull request, which
// shell/ci/release/dryrun.sh and shell/lib/release.sh did before.

package release

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/getoutreach/devbase/v2/internal/release/forge"
)

// These are the errors that Preview returns.
var (
	// ErrPreviewConflict is returned when the pull request does not merge
	// cleanly into the base branch.
	ErrPreviewConflict = errors.New("cannot preview the pull request: merge conflict")

	// ErrPreviewBase is returned when the base branch is not on the
	// remote, or has no common history with the pull request.
	ErrPreviewBase = errors.New("cannot preview the pull request")
)

// previewAction is how Preview previews a pull request.
type previewAction string

// These are the preview actions.
const (
	// previewSquash squashes the pull request onto the base branch.
	previewSquash previewAction = "squash"

	// previewMerged previews the base branch as it is, because the
	// changes of the pull request are already in it. A skip would let a
	// blocked release, such as a major bump, pass the check.
	previewMerged previewAction = "merged"

	// previewNone means that the pull request adds no change. A pull
	// request that was merged with a merge commit also gives this
	// result, because its head is an ancestor of the base branch.
	previewNone previewAction = "none"
)

// PreviewOptions are the inputs of Preview.
type PreviewOptions struct {
	// Base is the branch to preview against. Empty means the default
	// branch of the remote, or the release branch for a stable promotion.
	Base string

	// Head is the pull request ref. Empty means HEAD.
	Head string

	// Remote is the git URL to fetch from.
	Remote string

	// NewLinks makes the links in the release notes.
	NewLinks func(repoURL string) forge.NoteLinks

	// Log receives progress messages.
	Log io.Writer
}

// Preview previews the release that a pull request makes when it is
// merged: it squashes the pull request onto the base branch in a
// temporary worktree, as "Devbase CI", and plans the release of the
// result. The checkout does not change. The plan is nil when the pull
// request adds no change.
func Preview(ctx context.Context, r *Repo, opts *PreviewOptions) (*Plan, error) {
	log := logOr(opts.Log)
	g, head := r.Git, opts.Head
	if head == "" {
		head = "HEAD"
	}
	base := opts.Base
	if base == "" {
		var err error
		if base, err = r.previewBase(ctx, opts.Remote, head); err != nil {
			return nil, err
		}
	}
	if err := checkBranchName(ctx, g, base); err != nil {
		return nil, err
	}
	baseRef := "refs/remotes/origin/" + base
	if err := g.fetch(ctx, "--tags", "--", opts.Remote, "+refs/heads/"+base+":"+baseRef); err != nil {
		return nil, fmt.Errorf("%w: the branch %q is not on the remote: %w", ErrPreviewBase, base, err)
	}
	r.Git.FetchNotes(ctx, opts.Remote)
	if _, err := g.Run(ctx, "merge-base", baseRef, head); err != nil {
		return nil, fmt.Errorf("%w: %s and %s have no merge base; is the clone shallow? %w", ErrPreviewBase, base, head, err)
	}

	action, tree, err := decidePreview(ctx, g, baseRef, head)
	if err != nil || action == previewNone {
		return nil, err
	}
	commit := baseRef
	if action == previewSquash {
		fmt.Fprintf(log, "Squashing %s onto %s\n", head, base)
		msg, err := g.Run(ctx, "log", "--reverse", "--format=%B", baseRef+".."+head)
		if err != nil {
			return nil, err
		}
		commit, err = g.Run(ctx, "-c", "user.name=Devbase CI", "-c", "user.email=devbase@outreach.io",
			"commit-tree", tree, "-p", baseRef, "-m", msg)
		if err != nil {
			return nil, err
		}
	} else {
		fmt.Fprintf(log, "The changes of %s are already in %s, previewing %s as it is\n", head, base, base)
	}
	return r.planWorktree(ctx, commit, &PlanOptions{Branch: base, NewLinks: opts.NewLinks, DryRun: true})
}

// previewBase returns the branch to preview head against, as
// resolve_release_base_branch did: the release branch of the config for
// a stable promotion, which is a branch made from an rc tag and so an
// ancestor of the prerelease branch, or else the default branch of the
// remote.
func (r *Repo) previewBase(ctx context.Context, url, head string) (string, error) {
	out, err := r.Git.Run(ctx, "ls-remote", "--symref", "--", url, "HEAD")
	if err != nil {
		return "", err
	}
	def, _, ok := strings.Cut(strings.TrimPrefix(out, "ref: refs/heads/"), "\tHEAD")
	if !ok || def == out {
		return "", fmt.Errorf("%w: the remote has no default branch", ErrPreviewBase)
	}
	ro, err := loadServiceOptions(r.Git.Dir)
	release, pre := releaseBranches(r.Config.Branches)
	if err != nil || !ro.EnablePrereleases || release == nil || len(pre) != 1 {
		return def, err
	}
	preRef := "refs/remotes/origin/" + pre[0].Name
	if err := r.Git.fetch(ctx, "--", url, "+refs/heads/"+pre[0].Name+":"+preRef); err != nil {
		return "", fmt.Errorf("fetch %s: %w", pre[0].Name, err)
	}
	if r.Git.IsAncestor(ctx, head, preRef) {
		return release.Name, nil
	}
	return def, nil
}

// decidePreview decides how to preview head against base with git
// merge-tree, which needs git 2.38 or later. tree is the merged tree for
// previewSquash.
func decidePreview(ctx context.Context, g *Git, base, head string) (action previewAction, tree string, err error) {
	// git merge-tree also exits 1 for a ref that does not exist, which is
	// not a conflict.
	for _, ref := range []string{base, head} {
		if _, err := g.Run(ctx, "rev-parse", "--verify", "--quiet", ref+"^{commit}"); err != nil {
			return "", "", fmt.Errorf("%w: %q is not a commit", ErrPreviewBase, ref)
		}
	}
	out, err := g.Run(ctx, "merge-tree", "--write-tree", base, head)
	var exit *exec.ExitError
	switch {
	case errors.As(err, &exit) && exit.ExitCode() == 1:
		return "", "", fmt.Errorf("%w: %s onto %s: %w", ErrPreviewConflict, head, base, err)
	case err != nil:
		return "", "", fmt.Errorf("merge %s onto %s (git merge-tree needs git 2.38 or later): %w", head, base, err)
	}
	tree, _, _ = strings.Cut(out, "\n")
	baseTree, err := g.Run(ctx, "rev-parse", base+"^{tree}")
	if err != nil {
		return "", "", err
	}
	if tree != baseTree {
		return previewSquash, tree, nil
	}
	if headHasNetChanges(ctx, g, base, head) {
		return previewMerged, "", nil
	}
	return previewNone, "", nil
}

// headHasNetChanges reports whether head changes the tree since its fork
// point from base. After a merge that adds nothing, this means that the
// changes of head are already in base, by a squash or rebase merge. As in
// release.sh, it fails closed: when git cannot tell, it returns true, so
// that the preview runs the check.
func headHasNetChanges(ctx context.Context, g *Git, base, head string) bool {
	fork, err := g.Run(ctx, "merge-base", base, head)
	if err != nil {
		return true
	}
	_, err = g.Run(ctx, "diff", "--quiet", fork, head)
	return err != nil
}

// planWorktree plans the release of commit on a branch in a temporary
// worktree, so the config of commit applies and the checkout does not
// change.
func (r *Repo) planWorktree(ctx context.Context, commit string, opts *PlanOptions) (*Plan, error) {
	tmp, err := os.MkdirTemp("", "devbase-release-preview-")
	if err != nil {
		return nil, fmt.Errorf("create the preview worktree: %w", err)
	}
	defer os.RemoveAll(tmp)
	dir := filepath.Join(tmp, "worktree")
	if _, err := r.Git.Run(ctx, "worktree", "add", "--detach", "--", dir, commit); err != nil {
		return nil, err
	}
	defer r.Git.Run(context.WithoutCancel(ctx), "worktree", "remove", "--force", "--", dir) //nolint:errcheck // Why: cleanup.
	repo, err := LoadRepo(dir)
	if err != nil {
		return nil, err
	}
	return MakePlan(ctx, repo, opts)
}

// checkBranchName returns an error when name is not a valid branch name.
func checkBranchName(ctx context.Context, g *Git, name string) error {
	if _, err := g.Run(ctx, "check-ref-format", "--branch", name); err != nil {
		return fmt.Errorf("%w: invalid branch name %q", ErrPreviewBase, name)
	}
	return nil
}
