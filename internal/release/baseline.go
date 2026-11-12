// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Tags the current version of the release units that have
// no tag yet, so that their automatic releases start at the current
// commit and the first run releases nothing.

package release

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/Masterminds/semver/v3"
	"github.com/getoutreach/devbase/v2/internal/release/spec"
)

// ErrBaseline is returned when the baseline command cannot tag a unit.
var ErrBaseline = errors.New("cannot make the baseline tag")

// BaselineOptions are the inputs of Baseline.
type BaselineOptions struct {
	// Branch is the release branch that HEAD is on.
	Branch string

	// Units limits the baseline to these units. Empty means all units.
	Units []string

	// Version is the released version of a unit without a manifest
	// version, such as a Go module. It needs exactly one unit in Units.
	Version string

	// Remote is the git URL to fetch from and push to. Empty means that
	// the tags stay local.
	Remote string

	// Env is the environment of git.
	Env []string

	// DryRun prints the tags and makes none.
	DryRun bool

	// Log receives progress messages.
	Log io.Writer
}

// Baseline tags HEAD with the current version of each independent unit
// that has no tag, and adds the git notes of the branch channel. An npm
// unit uses the version in its package.json, which must be a released
// version; another kind uses opts.Version. A unit that has a tag, or
// that mirrors the root version, is skipped. It publishes nothing, so
// that the next plan releases a unit only for the commits after HEAD. A
// unit that has a tag keeps it: the next plan releases its commits after
// that tag, which can be commits before HEAD.
// HEAD must be the remote branch head, so that the tags are on the
// branch. It returns the tags that it made, or would make.
func Baseline(ctx context.Context, r *Repo, opts *BaselineOptions) (_ []string, err error) {
	ctx, end := startStep(ctx, "baseline")
	defer end(&err)
	log := logOr(opts.Log)
	branch := r.Config.Branch(opts.Branch)
	switch {
	case branch == nil:
		return nil, fmt.Errorf("%w: %q is not a release branch", ErrBaseline, opts.Branch)
	case opts.Version != "" && len(opts.Units) != 1:
		return nil, fmt.Errorf("%w: --version needs exactly one --unit", ErrBaseline)
	}
	for _, name := range opts.Units {
		if r.Config.Unit(name) == nil {
			return nil, fmt.Errorf("%w: %q", ErrUnknownUnit, name)
		}
	}
	g := r.Git
	head, err := g.Run(ctx, "rev-parse", "HEAD")
	if err != nil {
		return nil, err
	}
	if opts.Remote != "" {
		if err := g.fetch(ctx, "--tags", "--", opts.Remote); err != nil {
			return nil, fmt.Errorf("fetch: %w", err)
		}
		g.FetchNotes(ctx, opts.Remote)
		remote, err := g.RemoteHead(ctx, opts.Remote, branch.Name)
		if err != nil {
			return nil, err
		}
		if remote != head {
			return nil, fmt.Errorf("%w: HEAD %s is not the head %s of the remote branch %s", ErrBaseline, head, remote, branch.Name)
		}
	}

	var tags []string
	for i := range r.Config.Units {
		u := &r.Config.Units[i]
		if len(opts.Units) > 0 && !slices.Contains(opts.Units, u.Name) {
			continue
		}
		tag, err := r.baselineTag(ctx, u, opts, log)
		if err != nil {
			return nil, fmt.Errorf("unit %s: %w", u.Name, err)
		}
		if tag != "" {
			tags = append(tags, tag)
		}
	}
	switch {
	case len(tags) == 0:
		fmt.Fprintln(log, "Every unit has a tag; nothing to do")
		return nil, nil
	case opts.DryRun:
		fmt.Fprintf(log, "Dry run: would tag %s as\n  %s\n", head, strings.Join(tags, "\n  "))
		return tags, nil
	}
	if err := tagAndPush(ctx, g, opts.Env, opts.Remote, head, branch.Channel, tags); err != nil {
		return nil, err
	}
	fmt.Fprintf(log, "Tagged %s as\n  %s\n", head, strings.Join(tags, "\n  "))
	return tags, nil
}

// baselineTag returns the baseline tag of u, or "" when u needs none.
func (r *Repo) baselineTag(ctx context.Context, u *spec.Unit, opts *BaselineOptions, log io.Writer) (string, error) {
	if u.Strategy != spec.StrategyIndependent {
		fmt.Fprintf(log, "Skip %s: it takes the version of the root release\n", u.Name)
		return "", nil
	}
	existing, err := r.Git.FormatTags(ctx, "", u.TagFormat)
	if err != nil {
		return "", err
	}
	if len(existing) > 0 {
		fmt.Fprintf(log, "Skip %s: it has a tag, such as %s; the next plan releases its commits after its last tag\n",
			u.Name, existing[0].Name)
		return "", nil
	}
	version := opts.Version
	if u.NPM != nil {
		if version != "" {
			return "", fmt.Errorf("%w: an npm unit uses the version in its package.json, not --version", ErrBaseline)
		}
		if version, err = manifestVersion(r.Git.Dir, u); err != nil {
			return "", err
		}
		if !isReleaseVersion(version) {
			return "", fmt.Errorf("%w: the version %q in %s/package.json is not a released version; "+
				"set it to the version on the registry first", ErrBaseline, version, u.NPM.PkgRoot)
		}
	} else if version == "" {
		return "", fmt.Errorf("%w: give its released version with --unit %s --version <version>", ErrBaseline, u.Name)
	}
	v, err := semver.StrictNewVersion(version)
	if err != nil {
		return "", fmt.Errorf("%w: version %q: %w", ErrBaseline, version, err)
	}
	if u.Go != nil {
		if err := spec.CheckGoMajor(u.Go.ModulePath, v); err != nil {
			return "", err
		}
	}
	return u.TagFormat.Render(v.String()), nil
}
