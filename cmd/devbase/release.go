// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: release CLI subcommand (proof of concept for the
// devbase release RFC).

package main

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/getoutreach/devbase/v2/internal/release"
	"github.com/getoutreach/devbase/v2/internal/release/goreleaser"
	"github.com/urfave/cli/v3"
)

// These are the errors that the release command returns.
var (
	// errNoGoReleaser is returned by build for a repo without .goreleaser.yml.
	errNoGoReleaser = errors.New("the repository has no " + goreleaser.File)
)

// planFlag is the --plan flag of build and publish.
func planFlag() *cli.StringFlag {
	return &cli.StringFlag{Name: "plan", Usage: "Plan file from 'devbase release plan'", Value: "plan.json"}
}

// distFlag is the --dist flag of the commands that build or publish.
func distFlag() *cli.StringFlag {
	return &cli.StringFlag{Name: "dist", Usage: "Output directory", Value: "dist"}
}

// newReleaseCommand returns the "release" command and its subcommands.
func newReleaseCommand() *cli.Command {
	return &cli.Command{
		Name:  "release",
		Usage: "Plan, build and publish releases (proof of concept)",
		Commands: append([]*cli.Command{
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
		}, publishCommands()...),
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
	_, plan, err := makePlan(ctx, c.String("branch"))
	if err != nil {
		return err
	}
	if out := c.String("out"); out != "" {
		if err := release.WritePlan(out, plan); err != nil {
			return err
		}
	}

	printPlan(c, plan)
	return nil
}

// printPlan prints the next version and the release notes of a plan, or
// why it has no release.
func printPlan(c *cli.Command, plan *release.Plan) {
	w := c.Root().Writer
	if !plan.Release {
		fmt.Fprintf(w, "No release: %s\n", plan.Reason)
		return
	}
	last := "none"
	if plan.LastRelease != nil {
		last = plan.LastRelease.GitTag
	}
	fmt.Fprintf(w, "Last release: %s\nNext release: %s (%s)\n\n%s", last, plan.NextRelease.GitTag, plan.NextRelease.Type, plan.Notes)
}

// releaseBuild is the Action for "release build".
func releaseBuild(ctx context.Context, c *cli.Command) error {
	repo, plan, err := loadPlan(c)
	if err != nil || plan == nil {
		return err
	}
	if repo.Config.Artifacts == nil {
		return errNoGoReleaser
	}
	archives, err := release.Build(ctx, repo.Config.Artifacts, plan, &release.BuildOptions{
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
