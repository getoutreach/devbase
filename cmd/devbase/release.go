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
	"github.com/getoutreach/devbase/v2/internal/release/native"
	"github.com/getoutreach/devbase/v2/internal/release/rlog"
	"github.com/urfave/cli/v3"
)

// These are the errors that the release command returns.
var (
	// errNoBuild is returned by build for a release that builds nothing.
	errNoBuild = errors.New("the release configuration has no build (" + goreleaser.File +
		" or the build key of the " + native.Key + " section of " + native.File + ")")
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
// The output and the errors of the subcommands have the secrets of the
// environment masked, as semantic-release masks its output.
func newReleaseCommand() *cli.Command {
	// Before sets masker, so that other devbase commands do not build it.
	var masker *rlog.Masker
	cmd := &cli.Command{
		Name:  "release",
		Usage: "Plan, build and publish releases (proof of concept)",
		Flags: []cli.Flag{
			&cli.BoolFlag{Name: "debug", Usage: "Print each git and go command and each forge HTTP request"},
		},
		Before: func(ctx context.Context, c *cli.Command) (context.Context, error) {
			masker = rlog.NewMasker(os.Environ())
			root := c.Root()
			root.Writer, root.ErrWriter = masker.Writer(root.Writer), masker.Writer(root.ErrWriter)
			ctx = rlog.WithMasker(ctx, masker)
			if c.Bool("debug") {
				ctx = rlog.WithDebug(ctx, root.ErrWriter)
			}
			return ctx, nil
		},
		Commands: append([]*cli.Command{
			{
				Name:  "plan",
				Usage: "Compute the next version and release notes",
				Flags: []cli.Flag{
					&cli.StringFlag{Name: "branch", Usage: "Branch to plan for (default: the current branch)"},
					&cli.StringFlag{Name: "out", Usage: "Write the plan as JSON to this file"},
					dryRunFlag("Plan a release that publish refuses; hooks see options.dryRun as true"),
					&cli.BoolFlag{Name: "run-hooks", Usage: "Run the generateNotesCmd hooks and add their output to the notes"},
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
	for _, sub := range cmd.Commands {
		sub.Action = maskErrors(&masker, sub.Action)
	}
	return cmd
}

// maskErrors returns action with the secrets of *m masked in its error
// message, because gobox prints the error. errors.Is and errors.As still
// see the error.
func maskErrors(m **rlog.Masker, action cli.ActionFunc) cli.ActionFunc {
	return func(ctx context.Context, c *cli.Command) error {
		if err := action(ctx, c); err != nil {
			return &maskedError{msg: (*m).Mask(err.Error()), err: err}
		}
		return nil
	}
}

// maskedError is an error with the secrets masked in its message.
type maskedError struct {
	msg string
	err error
}

func (e *maskedError) Error() string { return e.msg }

func (e *maskedError) Unwrap() error { return e.err }

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
	_, plan, err := makePlan(ctx, c.Root().ErrWriter, &release.PlanOptions{
		Branch: c.String("branch"), DryRun: c.Bool("dry-run"), RunHooks: c.Bool("run-hooks"),
	})
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
		return errNoBuild
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
