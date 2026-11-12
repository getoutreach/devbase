// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Promotes the latest release candidate to the release
// branch, which dtss-tools did with a force push.

package release

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/getoutreach/devbase/v2/internal/release/spec"
)

// These are the errors that Promote returns.
var (
	// ErrNoCandidate is returned when no rc tag is newer than the last
	// stable release.
	ErrNoCandidate = errors.New("no release candidate is newer than the last stable release")

	// ErrNotCandidate is returned for a --tag that is not a release
	// candidate of the prerelease branch.
	ErrNotCandidate = errors.New("the tag is not a release candidate")
)

// PromoteOptions are the inputs of Promote.
type PromoteOptions struct {
	// Tag is the rc tag to promote. Empty means the latest one that is
	// newer than the last stable release.
	Tag string

	// Remote is the git URL to push to, with credentials.
	Remote string

	// DryRun prints the promotion and pushes nothing.
	DryRun bool

	// Log receives progress messages.
	Log io.Writer
}

// Promote moves the release branch to the commit of a release candidate
// with a force push, as dt release promote does. The push starts the
// stable release on the release branch. It uses --force-with-lease, so
// it fails when the remote release branch moved while it ran.
func Promote(ctx context.Context, r *Repo, opts *PromoteOptions) (_ *Tag, err error) {
	ctx, end := startStep(ctx, "promote")
	defer end(&err)
	log := logOr(opts.Log)
	release, pre := releaseBranches(r.Config.Branches)
	if release == nil || len(pre) != 1 {
		return nil, fmt.Errorf("%w: promote needs one release branch and one prerelease branch", ErrNoCandidate)
	}
	// Only the tags are needed: rc tags come only from the prerelease
	// branch, and a stable tag on any branch blocks older candidates.
	g := r.Git
	if err := g.fetch(ctx, "--tags", "--", opts.Remote); err != nil {
		return nil, fmt.Errorf("fetch: %w", err)
	}
	tags, err := g.Tags(ctx, "")
	if err != nil {
		return nil, err
	}
	candidate, err := promoteCandidate(tags, pre[0].Prerelease, opts.Tag)
	if err != nil {
		return nil, err
	}
	commit, err := g.Run(ctx, "rev-parse", candidate.Name+"^{commit}")
	if err != nil {
		return nil, err
	}
	head, err := g.RemoteHead(ctx, opts.Remote, release.Name)
	if err != nil {
		return nil, err
	}
	fmt.Fprintf(log, "Promote %s (%s) to %s, which is at %s\n", candidate.Name, commit, release.Name, head)
	if opts.DryRun {
		return candidate, nil
	}
	lease := "--force-with-lease=refs/heads/" + release.Name + ":" + head
	if _, err := g.Run(ctx, "push", lease, "--", opts.Remote, commit+":refs/heads/"+release.Name); err != nil {
		return nil, fmt.Errorf("push %s: %w", release.Name, err)
	}
	return candidate, nil
}

// releaseBranches returns the first release branch and the prerelease
// branches.
func releaseBranches(branches []spec.Branch) (release *spec.Branch, pre []spec.Branch) {
	for i, b := range branches {
		switch {
		case b.IsPrerelease():
			pre = append(pre, b)
		case release == nil:
			release = &branches[i]
		}
	}
	return release, pre
}

// promoteCandidate returns the rc tag to promote: tag, or the highest
// prerelease version with the prerelease id that is newer than every
// stable version.
func promoteCandidate(tags []Tag, prerelease, tag string) (*Tag, error) {
	var stable, best *Tag
	for i := range tags {
		t := &tags[i]
		switch {
		case t.Version.Prerelease() == "":
			if stable == nil || t.Version.GreaterThan(stable.Version) {
				stable = t
			}
		case hasPrereleaseID(t.Version, prerelease):
			if tag != "" && t.Name == tag {
				return t, nil
			}
			if best == nil || t.Version.GreaterThan(best.Version) {
				best = t
			}
		}
	}
	switch {
	case tag != "":
		return nil, fmt.Errorf("%w: %s", ErrNotCandidate, tag)
	case best == nil || stable != nil && !best.Version.GreaterThan(stable.Version):
		return nil, ErrNoCandidate
	}
	return best, nil
}
