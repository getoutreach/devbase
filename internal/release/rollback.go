// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Removes the git and forge parts of a release that failed
// part way, so that the next run releases the version again.

package release

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/getoutreach/devbase/v2/internal/release/forge"
)

// ErrRollbackRefused is returned when rollback stops before it deletes
// anything. The error lists the reasons.
var ErrRollbackRefused = errors.New("rollback refused")

// RollbackOptions are the inputs of Rollback.
type RollbackOptions struct {
	// Tag is the release tag to remove, such as v1.5.0.
	Tag string

	// Remote is the git URL to push to, with credentials.
	Remote string

	// Forge is the forge. It is used only when the config has a forge
	// plugin.
	Forge forge.Forge

	// Env is the environment of the registry CLIs.
	Env []string

	// Force removes a published forge release.
	Force bool

	// DryRun prints what rollback would delete and deletes nothing.
	DryRun bool

	// Log receives progress messages.
	Log io.Writer
}

// Rollback removes the forge release, the tag and the git note of a
// release (requirement 13). It stops, and deletes nothing, when a
// registry package exists for the version, when the release made a
// release commit, or when the forge release is published
// and opts.Force is false. It returns what it deleted, or would delete
// for a dry run.
func Rollback(ctx context.Context, r *Repo, opts *RollbackOptions) (_ []string, err error) {
	ctx, end := startStep(ctx, "rollback")
	defer end(&err)
	log := logOr(opts.Log)
	if opts.Tag == unstableTag {
		return nil, fmt.Errorf("%w: the unstable release replaces the %q tag on every run", ErrRollbackRefused, unstableTag)
	}
	v, err := ParseVersion(strings.TrimPrefix(opts.Tag, "v"))
	if err != nil || !strings.HasPrefix(opts.Tag, "v") {
		return nil, fmt.Errorf("%w: %q is not a release tag", ErrRollbackRefused, opts.Tag)
	}
	version := v.String()
	if r.Config.Forge() != nil && opts.Forge == nil {
		return nil, ErrNoForge
	}

	g := r.Git
	if err := g.fetch(ctx, "--tags", "--", opts.Remote); err != nil {
		return nil, fmt.Errorf("fetch: %w", err)
	}
	tagRef, notes := "refs/tags/"+opts.Tag, noteRef(opts.Tag)
	remote, err := g.Run(ctx, "ls-remote", "--", opts.Remote, tagRef, notes)
	if err != nil {
		return nil, err
	}
	var rel *forge.Release
	if r.Config.Forge() != nil {
		if rel, err = opts.Forge.FindRelease(ctx, opts.Tag); err != nil {
			return nil, err
		}
	}

	reasons, err := rollbackRefusals(ctx, r, opts, rel, version, log)
	if err != nil {
		return nil, err
	}
	if len(reasons) > 0 {
		return nil, fmt.Errorf("%w for %s:\n  %s", ErrRollbackRefused, opts.Tag, strings.Join(reasons, "\n  "))
	}

	var deleted, pushRefs []string
	if rel != nil {
		deleted = append(deleted, fmt.Sprintf("%s %s", opts.Forge.ReleaseName(), rel.URL))
	}
	for _, ref := range []string{tagRef, notes} {
		if strings.Contains(remote, "\t"+ref) {
			deleted = append(deleted, "remote "+ref)
			pushRefs = append(pushRefs, ":"+ref)
		}
	}
	if len(deleted) == 0 {
		fmt.Fprintf(log, "Nothing to roll back for %s\n", opts.Tag)
		return nil, nil
	}
	if opts.DryRun {
		fmt.Fprintf(log, "Dry run: would delete\n  %s\n", strings.Join(deleted, "\n  "))
		return deleted, nil
	}

	// The forge release goes first: without the tag, a forge such as
	// GitHub keeps the release as an untagged draft.
	if rel != nil {
		if err := opts.Forge.DeleteRelease(ctx, rel); err != nil {
			return nil, err
		}
	}
	if len(pushRefs) > 0 {
		if _, err := g.Run(ctx, append([]string{"push", "--", opts.Remote}, pushRefs...)...); err != nil {
			return nil, fmt.Errorf("delete the remote refs: %w", err)
		}
	}
	// The local refs are only a cache of the remote ones.
	//nolint:errcheck // Why: the tag may not be local.
	g.Run(ctx, "tag", "--delete", opts.Tag)
	//nolint:errcheck // Why: the note may not be local.
	g.Run(ctx, "update-ref", "-d", notes)
	fmt.Fprintf(log, "Deleted\n  %s\n", strings.Join(deleted, "\n  "))
	return deleted, nil
}

// rollbackRefusals returns the reasons that stop a rollback of version.
func rollbackRefusals(
	ctx context.Context, r *Repo, opts *RollbackOptions, rel *forge.Release, version string, log io.Writer,
) ([]string, error) {
	var reasons []string
	if rel != nil && !rel.Draft && !opts.Force {
		reasons = append(reasons, fmt.Sprintf("the %s %s is published; users can have its assets (use --force)",
			opts.Forge.ReleaseName(), rel.URL))
	}

	// A release commit is not removed: rollback does not rewrite history,
	// and a rerun of publish finishes that release instead.
	msg, err := r.Git.Run(ctx, "log", "-1", "--format=%H %s", "refs/tags/"+opts.Tag+"^{commit}", "--")
	if hash, subject, _ := strings.Cut(msg, " "); err == nil && subject == releaseCommitSubject(version) {
		reasons = append(reasons, fmt.Sprintf("the release made the release commit %s; rerun publish to finish the release", hash))
	}

	// A later run with the same version skips a registry that has it
	// (requirement 8), so the old package would stay.
	t := &regTarget{dir: r.Git.Dir, version: version, env: opts.Env, log: log}
	for i := range r.Config.Plugins {
		if pl := &r.Config.Plugins[i]; isRegistry(pl) {
			reg := registryOf(pl)
			found, err := reg.exists(ctx, t)
			if err != nil {
				return nil, err
			}
			if found {
				reasons = append(reasons, fmt.Sprintf("the %s package for %s exists", reg.name(), version))
			}
		}
	}
	return reasons, nil
}
