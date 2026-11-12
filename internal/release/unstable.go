// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: The unstable release on the prerelease branch, which
// shell/ci/release/pre-release.sh made before.

package release

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"

	"github.com/getoutreach/devbase/v2/internal/release/forge"
	"github.com/getoutreach/devbase/v2/internal/release/spec"
)

// unstableTag is the tag and the forge release of the unstable release.
const unstableTag = "unstable"

// unstableInclude is the script that repos run after the unstable release.
const unstableInclude = "scripts/unstable-release.include.sh"

// UnstableOptions are the inputs of Unstable.
type UnstableOptions struct {
	// Branch is the branch that CI runs on.
	Branch string

	// Dist is the build output folder, relative to the repository.
	Dist string

	// Remote is the git URL to push to, with credentials.
	Remote string

	// Forge publishes the unstable release.
	Forge forge.Forge

	// Env is the environment of the build and the include script.
	Env []string

	// DryRun stops after the checks, as pre-release.sh --dry-run does.
	DryRun bool

	// Log receives progress messages.
	Log io.Writer
}

// Unstable makes the unstable release of HEAD, as pre-release.sh does:
// it builds the archives with version 0.0.0-unstable+<commit>, replaces
// the forge release and the tag "unstable", and runs
// scripts/unstable-release.include.sh. It does nothing when service.yaml
// does not enable prereleases, or when CI is not on the prerelease branch.
func Unstable(ctx context.Context, r *Repo, opts *UnstableOptions) (err error) {
	ctx, end := startStep(ctx, "unstable")
	defer end(&err)
	log := logOr(opts.Log)
	if ok, err := r.UnstableEnabled(opts.Branch, opts.DryRun, log); !ok {
		return err
	}

	arts, err := r.unstableArtifacts()
	if err != nil {
		return err
	}
	if arts != nil {
		if err := unstableRelease(ctx, r, arts, opts, log); err != nil {
			return err
		}
	} else {
		fmt.Fprintf(log, "No .goreleaser.yml, skipping creating unstable release\n")
	}
	return runUnstableInclude(ctx, r.Git.Dir, opts.Env, log)
}

// UnstableEnabled reports whether Unstable makes a release on branch, as
// pre-release.sh checks it: service.yaml must enable prereleases, the
// branch must be the prerelease branch, and it must not be a dry run. It
// logs why it skips.
func (r *Repo) UnstableEnabled(branch string, dryRun bool, log io.Writer) (bool, error) {
	ro, err := loadServiceOptions(r.Git.Dir)
	switch {
	case err != nil:
		return false, err
	case !ro.EnablePrereleases:
		fmt.Fprintln(log, "releaseOptions.enablePrereleases is not true, skipping unstable release")
		return false, nil
	case dryRun:
		return false, nil
	case branch != ro.PrereleasesBranch:
		fmt.Fprintf(log, "the branch %q is not the prereleases branch %q, skipping prerelease\n", branch, ro.PrereleasesBranch)
		return false, nil
	}
	return true, nil
}

// unstableRelease builds the archives and replaces the unstable release.
func unstableRelease(ctx context.Context, r *Repo, arts *spec.Artifacts, opts *UnstableOptions, log io.Writer) error {
	plan, _, _, err := r.newPlan(ctx, opts.Branch)
	if err != nil {
		return err
	}
	version := "0.0.0-unstable+" + plan.Commit
	plan.Release = true
	plan.NextRelease = &PlanRelease{Version: version, GitTag: spec.RootTagFormat.Render(version)}
	plan.Targets = planTargets(arts)
	fmt.Fprintf(log, "Creating unstable release (v%s)\n", version)
	archives, err := Build(ctx, arts, plan, &BuildOptions{Dir: r.Git.Dir, Dist: opts.Dist, Env: opts.Env})
	if err != nil {
		return err
	}
	sums := filepath.Join(r.Git.Dir, opts.Dist, arts.Checksums)
	if err := WriteChecksums(sums, archives); err != nil {
		return err
	}

	// Replace the release and the tag. As in pre-release.sh, a release or
	// tag that is not there is not an error.
	f := opts.Forge
	if old, err := f.FindRelease(ctx, unstableTag); err == nil && old != nil {
		if err := f.DeleteRelease(ctx, old); err != nil {
			return err
		}
	}
	//nolint:errcheck // Why: the tag may not be local.
	r.Git.Run(ctx, "tag", "--delete", unstableTag)
	//nolint:errcheck // Why: the tag may not be on the remote.
	r.Git.Run(ctx, "push", "--delete", "--", opts.Remote, "refs/tags/"+unstableTag)
	if _, err := r.Git.Run(ctx, "tag", unstableTag, plan.Commit); err != nil {
		return err
	}
	if _, err := r.Git.Run(ctx, "push", "--", opts.Remote, "refs/tags/"+unstableTag); err != nil {
		return fmt.Errorf("push the %s tag: %w", unstableTag, err)
	}
	rel, err := f.CreateRelease(ctx, &forge.ReleaseSpec{
		Tag: unstableTag, Target: opts.Branch, Name: unstableTag, Prerelease: true, Draft: true, GenerateNotes: true,
	})
	if err != nil {
		return err
	}
	if rel, err = finishRelease(ctx, f, rel, append(archives, sums), false); err != nil {
		return err
	}
	fmt.Fprintf(log, "Published %s: %s\n", f.ReleaseName(), rel.URL)
	return nil
}

// runUnstableInclude runs scripts/unstable-release.include.sh, when the
// repository has it, with DRYRUN=false.
func runUnstableInclude(ctx context.Context, dir string, env []string, log io.Writer) error {
	script := filepath.Join(dir, unstableInclude)
	if _, err := os.Stat(script); errors.Is(err, os.ErrNotExist) {
		return nil
	}
	_, err := execCommand(ctx, log, &command{
		dir: dir, env: slices.Concat(envOrProcess(env), []string{"DRYRUN=false"}), name: script, display: unstableInclude,
	})
	return err
}
