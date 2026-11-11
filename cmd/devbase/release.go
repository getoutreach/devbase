// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: release CLI subcommand (proof of concept for the
// devbase release RFC).

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/getoutreach/devbase/v2/internal/release"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v3"
)

// These are the errors that the release command returns.
var (
	// errNoGoReleaser is returned by build for a repo without .goreleaser.yml.
	errNoGoReleaser = errors.New("the repository has no " + release.GoReleaserConfigFile)

	// errMissingAsset is returned by checksums when a build job did not
	// write an archive that the plan lists.
	errMissingAsset = errors.New("archive from the plan is missing")
)

// planFlag is the --plan flag of build and checksums.
func planFlag() *cli.StringFlag {
	return &cli.StringFlag{Name: "plan", Usage: "Plan file from 'devbase release plan'", Value: "plan.json"}
}

// distFlag is the --dist flag of build and checksums.
func distFlag() *cli.StringFlag {
	return &cli.StringFlag{Name: "dist", Usage: "Output directory", Value: "dist"}
}

// newReleaseCommand returns the "release" command and its subcommands.
func newReleaseCommand() *cli.Command {
	return &cli.Command{
		Name:  "release",
		Usage: "Plan and build releases (proof of concept)",
		Commands: []*cli.Command{
			{
				Name:  "plan",
				Usage: "Compute the next version and release notes",
				Flags: []cli.Flag{
					&cli.StringFlag{Name: "branch", Usage: "Branch to plan for (default: the current branch)"},
					&cli.StringFlag{Name: "out", Usage: "Write the plan as JSON to this file"},
				},
				Action: releasePlan,
			},
			{
				Name:  "build",
				Usage: "Build binaries and archives for the targets of a plan",
				Flags: []cli.Flag{
					planFlag(),
					distFlag(),
					&cli.StringSliceFlag{Name: "target", Usage: "Target to build as os/arch (repeatable; default: all)"},
					&cli.BoolFlag{Name: "skip-before", Usage: "Skip the before hooks"},
				},
				Action: releaseBuild,
			},
			{
				Name:   "checksums",
				Usage:  "Write checksums.txt for the archives of all build jobs",
				Flags:  []cli.Flag{planFlag(), distFlag()},
				Action: releaseChecksums,
			},
		},
	}
}

// loadRepo loads the repository in the working directory.
func loadRepo() (*release.Repo, error) {
	dir, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("get working directory: %w", err)
	}
	return release.LoadRepo(dir)
}

// loadPlan loads the repository and the plan of the --plan flag. It
// returns a nil plan when the plan has no release.
func loadPlan(c *cli.Command) (*release.Repo, *release.Plan, error) {
	repo, err := loadRepo()
	if err != nil {
		return nil, nil, err
	}
	plan, err := release.ReadPlan(c.String("plan"), repo)
	if err != nil {
		return nil, nil, err
	}
	if !plan.Release {
		fmt.Fprintln(c.Root().Writer, "The plan has no release; nothing to do")
		return repo, nil, nil
	}
	return repo, plan, nil
}

// releasePlan is the Action for "release plan".
func releasePlan(ctx context.Context, c *cli.Command) error {
	repo, err := loadRepo()
	if err != nil {
		return err
	}
	for _, w := range repo.Config.Warnings {
		logrus.Warn(w)
	}
	plan, err := release.MakePlan(ctx, repo, &release.PlanOptions{
		Branch:   c.String("branch"),
		NewLinks: func(repoURL string) release.NoteLinks { return release.GitHubLinks{RepoURL: repoURL} },
	})
	if err != nil {
		return fmt.Errorf("plan release: %w", err)
	}
	if out := c.String("out"); out != "" {
		if err := release.WritePlan(out, plan); err != nil {
			return err
		}
	}

	w := c.Root().Writer
	if !plan.Release {
		fmt.Fprintf(w, "No release: %s\n", plan.Reason)
		return nil
	}
	last := "none"
	if plan.LastRelease != nil {
		last = plan.LastRelease.GitTag
	}
	fmt.Fprintf(w, "Last release: %s\nNext release: %s (%s)\n\n%s", last, plan.NextRelease.GitTag, plan.NextRelease.Type, plan.Notes)
	return nil
}

// releaseBuild is the Action for "release build".
func releaseBuild(ctx context.Context, c *cli.Command) error {
	repo, plan, err := loadPlan(c)
	if err != nil || plan == nil {
		return err
	}
	if repo.GoReleaser == nil {
		return errNoGoReleaser
	}
	archives, err := release.Build(ctx, repo.GoReleaser, plan, &release.BuildOptions{
		Dir:       repo.Git.Dir,
		Dist:      c.String("dist"),
		Targets:   c.StringSlice("target"),
		Env:       os.Environ(),
		SkipHooks: c.Bool("skip-before"),
	})
	if err != nil {
		return fmt.Errorf("build release: %w", err)
	}
	for _, a := range archives {
		fmt.Fprintln(c.Root().Writer, a)
	}
	return nil
}

// releaseChecksums is the Action for "release checksums". It runs once
// after all build jobs, and fails if an archive from the plan is missing.
func releaseChecksums(_ context.Context, c *cli.Command) error {
	_, plan, err := loadPlan(c)
	if err != nil || plan == nil {
		return err
	}
	dist := c.String("dist")
	var archives []string
	for _, asset := range plan.Assets {
		if asset == release.ChecksumsFile {
			continue
		}
		matches, err := filepath.Glob(filepath.Join(dist, "*", asset))
		if err != nil {
			return fmt.Errorf("find %s: %w", asset, err)
		}
		if len(matches) != 1 {
			return fmt.Errorf("%w: %s", errMissingAsset, asset)
		}
		archives = append(archives, matches[0])
	}
	out := filepath.Join(dist, release.ChecksumsFile)
	if err := release.WriteChecksums(out, archives); err != nil {
		return err
	}
	fmt.Fprintln(c.Root().Writer, out)
	return nil
}
