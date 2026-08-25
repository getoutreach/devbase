// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Lint GraphQL CLI subcommand.

package main

import (
	"context"
	"fmt"
	"os"

	"github.com/getoutreach/devbase/v2/internal/graphql/config"
	"github.com/getoutreach/devbase/v2/internal/graphql/lint"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v3"
)

// newLintGraphQLCommand returns the "graphql" subcommand of "lint",
// which runs GraphQL schema lint checks.
func newLintGraphQLCommand() *cli.Command {
	return &cli.Command{
		Name:      "graphql",
		Usage:     "Run GraphQL lint checks on the codebase",
		ArgsUsage: "[paths...]",
		Flags: []cli.Flag{
			&cli.StringSliceFlag{
				Name:    "exclude",
				Aliases: []string{"e"},
				Usage:   "Glob pattern to exclude (repeatable, additive with scripts/devbase.yaml)",
			},
		},
		Action: lintGraphQL,
	}
}

// lintGraphQL is the Action for the "graphql" subcommand. It resolves
// scripts/devbase.yaml for the current working directory, finds the
// *.graphql files under the given paths (the current directory if none
// are given), runs the Tier 1 rules against them, and prints any
// violation. It returns a non-zero exit code, with no further output,
// if any violation was found -- every Tier 1 rule is "error" severity,
// so a violation always fails the run.
func lintGraphQL(ctx context.Context, c *cli.Command) error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}

	lintConfig, configDir, err := config.Load(cwd)
	if err != nil {
		return fmt.Errorf("load scripts/devbase.yaml: %w", err)
	}

	excludes := lintConfig.MergeExcludes(c.StringSlice("exclude")...)
	logrus.WithFields(logrus.Fields{
		"configDir": configDir,
		"excludes":  excludes,
		"ruleCount": len(lintConfig.Rules),
	}).Debug("resolved graphql lint config")

	paths := c.Args().Slice()
	if len(paths) == 0 {
		paths = []string{"."}
	}

	files, err := lint.FindGraphQLFiles(paths, excludes)
	if err != nil {
		return fmt.Errorf("find graphql files: %w", err)
	}

	violations, err := lint.Files(files, lintConfig)
	if err != nil {
		return fmt.Errorf("lint graphql files: %w", err)
	}

	for _, v := range violations {
		if _, err := fmt.Fprintln(c.Writer, v.String()); err != nil {
			return fmt.Errorf("write violation: %w", err)
		}
	}
	if len(violations) > 0 {
		return cli.Exit("", 1)
	}

	return nil
}
